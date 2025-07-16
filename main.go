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
	statusConnStr = "user=myuser password=mypassword dbname=controlstatus sslmode=disable"
	codexConnStr  = "user=myuser password=mypassword dbname=codex sslmode=disable"
	evidencePath  = "/api/v1/evidence"
	acIDQuery     = `select internal_id from codex_acceptance_criterion where internal_id like 'CTOB%';`
	assetIDQuery  = `select asset_id from codex_assets where status != 'Retired' and status != 'Reassigned' order by asset_id;`
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
	mapping := getDataFromCsv()
	acIDMap := makeACIDMap(mapping)
	acIDs := getIDsFromCodex(acIDQuery, "internal_id")
	verifyMappingData(acIDMap, acIDs)

	assetIDs := getIDsFromCodex(buildGetAssetsIdQuery(), "asset_id")
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrencyLimits)

	log.Printf("start migrating evidences: assetIDs=%v", assetIDs)
	for _, assetID := range assetIDs {
		sem <- struct{}{} // Acquire a slot
		wg.Add(1)
		go func(assetID string) {
			defer wg.Done()
			defer func() { <-sem }() // Release the slot
			migrate(assetID, acIDMap)
		}(assetID)
	}

	wg.Wait()
	log.Println("migration completed")
	exportMigrationDataToExcel()
}

func migrate(assetID string, acIDMap map[string][]string) {
	client := &http.Client{}
	evidenceCreated := 0
	requiredCreate := 0
	for sourceAC, targetACs := range acIDMap {
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
					log.Printf("Error creating evidence: assetID=%s sourceAC=%s targetAC=%s error=%s", assetID, sourceAC, targetAC, err.Error())
					migrationResults = append(migrationResults, result)
					continue
				}
				evidenceCreated++
				result.Succeed = true
				migrationResults = append(migrationResults, result)
				log.Printf("Evidence created: assetID=%s sourceAC=%s targetAC=%s", assetID, sourceAC, targetAC)
			}
		}
	}

	log.Printf("evidence creation summary: assetID=%s created=%d required=%d", assetID, evidenceCreated, requiredCreate)
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
	// target ac must exist
	if len(data) == 0 {
		panic("no data found in the mapping file")
	}
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
	fmt.Println("mapping data is valid")
}

// readEvidences retrieves evidences for a given asset ID AC ID from local db.
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
	url := getEvidenceURL()

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
	for attempt := 1; attempt <= 5; attempt++ {
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

		resp.Body.Close()

		// Exponential backoff
		log.Printf("retrying post request %d", attempt+1)
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

// makeACIDMap creates a mapping from source AC ID to target AC IDs.
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
			log.Printf("failed to open Excel file: %s", err.Error())
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

func getEvidenceURL() string {
	var baseURL string
	switch env {
	case "prod":
		baseURL = "https://australia-southeast1-anz-x-xplore-prod-44f597.cloudfunctions.net/xp-cf-xplore-api"
	case "staging":
		baseURL = "https://australia-southeast1-anz-x-xplore-staging-1bbe6e.cloudfunctions.net/xp-cf-xplore-api"
	default:
		baseURL = "https://australia-southeast1-anz-x-xplore-np-4a74dd.cloudfunctions.net/xp-cf-xplore-api"
	}
	return baseURL + evidencePath
}

func buildGetAssetsIdQuery() string {
	return fmt.Sprintf("SELECT asset_id FROM codex_assets WHERE status != 'Retired' AND status != 'Reassigned' ORDER BY asset_id LIMIT %d OFFSET %d;", rowLimits, startingRow-1)
}
