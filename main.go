package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/xuri/excelize/v2"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	assetIDQuery = `select asset_id from codex_assets where status != 'Retired' and status != 'Reassigned' order by asset_id limit 20 offset 50;`
	dryRun       = false
	token        = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjY2MGVmM2I5Nzg0YmRmNTZlYmU4NTlmNTc3ZjdmYjJlOGMxY2VmZmIiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOiIzMjU1NTk0MDU1OS5hcHBzLmdvb2dsZXVzZXJjb250ZW50LmNvbSIsImF6cCI6IjExNDA2MjgwNzcyNjE2MTMyMTk3MCIsImVtYWlsIjoieHAtc2EteHBsb3JlLXRjbHN5bmNAYW56LXgteHBsb3JlLXByb2QtNDRmNTk3LmlhbS5nc2VydmljZWFjY291bnQuY29tIiwiZW1haWxfdmVyaWZpZWQiOnRydWUsImV4cCI6MTc0NzY2ODg0MiwiaWF0IjoxNzQ3NjY1MjQyLCJpc3MiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb20iLCJzdWIiOiIxMTQwNjI4MDc3MjYxNjEzMjE5NzAifQ.nb0UdHaxnny0RvEUTh1tvDz6g46rRZ93r3DVYawjJ7OT_HbhUDeX9UQH-rpK9OXSbTQNHz95hyarIKbVN6ToXc5-9ePmURt5JArIopu3lB6Q6ErdnA6PGnrc8MRssDw794RLEt9b8YWgPLkpeBp_pURPb2LmHMvB76aU5T4xcsJ_dLj9eV3oqHVKynyjlZ1nXe2aV-xofZw1QL_iL6BIH7WsZnhiPdRO_dSGx_-L5iMYStCdh0kUgcbpBbWT8ZWtphDun-XBvbTjn6FpDtNXQqIsXQO2UO8xWegWT0CKu52keiBaFDT2hPjp2fcc8ZXlE0rjp_3nIJk9L_HDNoAqmw"
)

const (
	// baseURL       = "https://australia-southeast1-anz-x-xplore-staging-1bbe6e.cloudfunctions.net/xp-cf-xplore-api"
	// baseURL    = "https://australia-southeast1-anz-x-xplore-np-4a74dd.cloudfunctions.net/xp-cf-xplore-api"
	statusConnStr = "user=myuser password=mypassword dbname=controlstatus sslmode=disable"
	codexConnStr  = "user=myuser password=mypassword dbname=codex sslmode=disable"
	baseURL       = "https://australia-southeast1-anz-x-xplore-prod-44f597.cloudfunctions.net/xp-cf-xplore-api"
	evidencePath  = "/api/v1/evidence"
	acIDQuery     = `select internal_id from codex_acceptance_criterion where internal_id like 'CTOB%';`
	assetsPath    = "/api/v1/assets"
	gciPath       = "/api/v1/generic-control-instances/"
)

var migrationResults []migrationResult

type migrationMapData struct {
	SourceControlID string
	SourceACID      string
	TargetControlID string
	TargetACID      string
}

type migrationResult struct {
	AssetID    string
	SourceACID string
	TargetACID string
	Succeed    bool
}

func main() {
	handler := slog.NewJSONHandler(os.Stdout, nil)
	slog.SetDefault(slog.New(handler))

	mapping := getDataFromCsv()
	acIDMap := makeACIDMap(mapping)
	acIDs := getIDsFromCodex(acIDQuery, "internal_id")
	verifyMappingData(acIDMap, acIDs)

	var wg sync.WaitGroup

	assetIDs := getIDsFromCodex(assetIDQuery, "asset_id")
	slog.Info("start migrating evidences", slog.Any("assetIDs", assetIDs))

	for _, assetID := range assetIDs {
		wg.Add(1)
		go func(assetID string) {
			defer wg.Done()
			migrate(assetID, acIDMap)
		}(assetID)
	}

	wg.Wait()
	slog.Info("migration completed")
	exportMigrationDataToExcel()
}

func migrate(assetID string, processedMap map[string][]string) {
	client := &http.Client{}
	evidenceCreated := 0
	requiredCreate := 0
	for sourceAC, targetACs := range processedMap {
		evidences := readEvidences(assetID, sourceAC)
		if len(evidences) == 0 {
			continue
		}
		for _, targetAC := range targetACs {
			result := migrationResult{
				AssetID:    assetID,
				SourceACID: sourceAC,
				TargetACID: targetAC,
			}
			for _, evidence := range evidences {
				requiredCreate++
				if dryRun {
					continue
				}
				updatedEvidence := evidence
				updatedEvidence.ControlId = getControlID(targetAC)
				updatedEvidence.ControlComponentId = targetAC
				err := createEvidence(client, updatedEvidence)
				if err != nil {
					slog.Error("Error creating evidence", slog.Any("assetID", assetID), slog.Any("sourceAC", sourceAC), slog.Any("targetAC", targetAC), slog.String("error", err.Error()))
					migrationResults = append(migrationResults, result)
					continue
				}
				evidenceCreated++
				result.Succeed = true
				migrationResults = append(migrationResults, result)
				slog.Info("Evidence created", slog.Any("assetID", assetID), slog.Any("sourceAC", sourceAC), slog.Any("targetAC", targetAC))
			}
		}
	}

	slog.Info("evidence creation summary",
		slog.String("assetID", assetID),
		slog.Int("created", evidenceCreated),
		slog.Int("required", requiredCreate),
	)
}

func getDataFromCsv() []migrationMapData {
	filePath := "mapping.xlsx"
	sheetName := "Sheet1"
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	rows, err := f.GetRows(sheetName)
	if err != nil {
		panic(err)
	}

	var results []migrationMapData
	for i, row := range rows {
		if i == 0 {
			continue
		}
		res := migrationMapData{
			SourceControlID: row[0],
			SourceACID:      row[1],
			TargetControlID: row[2],
			TargetACID:      row[3],
		}
		results = append(results, res)
	}

	return results
}

func verifyMappingData(data map[string][]string, acIDs []string) {
	// one evidence can be mapped to multiple tcl
	// not more than one tcl evidence should be created
	// ac must exist
	counter := make(map[string]int)
	for _, targetACs := range data {
		for _, targetAC := range targetACs {
			if !slices.Contains(acIDs, targetAC) {
				panic(fmt.Errorf("invalid ac ID found: %s", targetAC))
			}
			counter[targetAC]++
			if counter[targetAC] > 1 {
				panic(fmt.Errorf("multiple evidences found for ac ID: %s", targetAC))
			}
		}
	}
	slog.Info("mapping data is valid")
}

func readEvidences(assetID, acID string) []Evidence {
	db, err := sqlx.Open("postgres", statusConnStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var results []Evidence

	query := `
SELECT 
    s.asset_id, e.title, e.control_id, e.status, e.component_id, e.attachment_names, 
    e.content, e.control_component_id, e.evidence_type_id, 
    e.evidence_type_title, e.provided_at, e.provided_by, e.expires_at,
    e.control_type
FROM 
    evidences e 
JOIN 
    generic_control_statuses s 
ON 
    e.generic_control_status_id = s.id
WHERE 
    e.bot_id = '' 
    AND s.asset_id = $1 
    AND e.control_component_id = $2;
`
	err = db.Select(&results, query, assetID, acID)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	return results
}

func getIDsFromCodex(query, key string) []string {
	db, err := sqlx.Open("postgres", codexConnStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Queryx(query)
	if err != nil {
		log.Fatalln(err)
	}
	defer rows.Close()

	result := make([]string, 0)
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			log.Fatalln(err)
		}
		result = append(result, row[key].(string))
	}
	return result
}

func createEvidence(client *http.Client, evidence Evidence) error {
	// in case controls API crushes
	url := baseURL + evidencePath

	resp, err := sendPOSTRequest(client, url, evidence)
	if err != nil {
		return fmt.Errorf("failed to send POST request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create evidence: %s", body)
	}
	return nil
}

func sendPOSTRequest[T any](client *http.Client, url string, payload T) (*http.Response, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	var resp *http.Response
	for attempt := 1; attempt <= 4; attempt++ {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode < 500 {
			return resp, nil
		}

		// Close the response body to avoid resource leaks
		resp.Body.Close()

		// Exponential backoff
		slog.Info("retrying request", slog.Int("attempt", attempt))
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	return nil, fmt.Errorf("request failed after retries: status %d", resp.StatusCode)
}

func sendGET[T any](client *http.Client, url string) (*T, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshaling failed: %w", err)
	}

	return &result, nil
}

func getControlID(acID string) string {
	return strings.Split(acID, ".")[0]
}

func makeACIDMap(data []migrationMapData) map[string][]string {
	acIDMap := make(map[string][]string)
	for _, item := range data {
		sourceACID := item.SourceControlID + "." + item.SourceACID
		targetACID := item.TargetControlID + "." + item.TargetACID
		acIDMap[sourceACID] = append(acIDMap[sourceACID], targetACID)
	}
	return acIDMap
}

func exportMigrationDataToExcel() {
	var f *excelize.File
	var err error
	filePath := "migration_results.xlsx"

	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		f = excelize.NewFile()
	} else {
		f, err = excelize.OpenFile(filePath)
		if err != nil {
			slog.Error("failed to open Excel file", slog.String("error", err.Error()))
		}
	}
	defer f.Close()

	sheet := f.GetSheetName(f.GetActiveSheetIndex())

	rows, err := f.GetRows(sheet)
	rowsLength := len(rows)
	if err != nil {
		slog.Error("failed to get sheet rows")
	}

	if len(rows) == 0 {
		headers := []string{"Asset ID", "Source ac", "Target AC", "Succeed"}
		for col, header := range headers {
			cell, _ := excelize.CoordinatesToCellName(col+1, 1)
			f.SetCellValue(sheet, cell, header)
		}
		rowsLength++
	}

	// Find first empty row
	startRow := rowsLength + 1

	for idx, r := range migrationResults {
		row := startRow + idx
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), r.AssetID)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), r.SourceACID)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), r.TargetACID)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), r.Succeed)
	}

	if err := f.SaveAs(filePath); err != nil {
		slog.Error("failed to save Excel file")
	}
}
