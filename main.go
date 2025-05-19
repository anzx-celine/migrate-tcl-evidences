package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/xuri/excelize/v2"
)

const (
	dryRun        = true
	statusConnStr = "user=myuser password=mypassword dbname=controlstatus sslmode=disable"
	codexConnStr  = "user=myuser password=mypassword dbname=codex sslmode=disable"
	// baseURL       = "https://australia-southeast1-anz-x-xplore-staging-1bbe6e.cloudfunctions.net/xp-cf-xplore-api"
	// baseURL    = "https://australia-southeast1-anz-x-xplore-np-4a74dd.cloudfunctions.net/xp-cf-xplore-api"
	baseURL      = "https://australia-southeast1-anz-x-xplore-prod-44f597.cloudfunctions.net/xp-cf-xplore-api"
	assetsPath   = "/api/v1/assets"
	gciPath      = "/api/v1/generic-control-instances/"
	evidencePath = "/api/v1/evidence"
	assetIDQuery = `select asset_id from codex_assets where status != 'Retired' and status != 'Reassigned' order by asset_id limit 5 offset 0;`
	acIDQuery    = `select internal_id from codex_acceptance_criterion where internal_id like 'CTOB%';`
	token        = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImUxNGMzN2Q2ZTVjNzU2ZThiNzJmZGI1MDA0YzBjYzM1NjMzNzkyNGUiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOiIzMjU1NTk0MDU1OS5hcHBzLmdvb2dsZXVzZXJjb250ZW50LmNvbSIsImF6cCI6IjEwNzgyNDY5NTM0ODI3NzEyNzI2MCIsImVtYWlsIjoieHAtc2EteHBsb3JlLXRjbHN5bmNAYW56LXgteHBsb3JlLXN0YWdpbmctMWJiZTZlLmlhbS5nc2VydmljZWFjY291bnQuY29tIiwiZW1haWxfdmVyaWZpZWQiOnRydWUsImV4cCI6MTc0NzI3NDQxOSwiaWF0IjoxNzQ3MjcwODE5LCJpc3MiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb20iLCJzdWIiOiIxMDc4MjQ2OTUzNDgyNzcxMjcyNjAifQ.aEnJumEmcUbkh80h5kDTl9YmASl0ss9qmUvjPIcG0NX-gyFgEsBIaGWTe3LsW_INc_6DADbgh63wauOUU6IebxSTpMnn7oBJgIUi-vXKppC5XBj422ZHhNdzlHKeYoCqJpri0C0FiZ6HLneoHKkWU_MYcwt_bVxbxPv4qBE_EPA4Yn93xWxBtfK0XKpuQqigJSrUJYzO3WDrx70LNNXKb2_jPQ3MThgEAr5s2C-f0ZLMPsJtIgQ9gTMiGdW28MQ0Thws1yp6EYD6W7logjHLnn9MJXncmxNH80kdw5_zqtLvvDNp-NZYnsj6V-RnMVp6_1EgFOL__OT4yBXwm8wQ5g"
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
	assetIDs := getIDsFromCodex(assetIDQuery, "asset_id")
	// this is used for staging test
	// stagingDummyData := acIDMap["PAC-001.1"]
	// stagingDummyMap := make(map[string][]string)
	// stagingDummyMap["PAC-001.1"] = stagingDummyData
	// assetIDs := []string{"xplore"}
	now := time.Now()
	fmt.Printf("Start time: %s\n", now.Format(time.RFC3339))
	migrate(assetIDs, acIDMap)
	fmt.Printf("Migration took: %s\n", time.Since(now))
	exportMigrationDataToExcel("migration_result.xlsx")
}

func migrate(assetIDs []string, processedMap map[string][]string) {
	client := &http.Client{}
	evidenceCreated := 0
	requiredCreate := 0
	for _, assetID := range assetIDs {
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
						migrationResults = append(migrationResults, result)
						continue
					}
					updatedEvidence := evidence
					updatedEvidence.ControlId = getControlID(targetAC)
					updatedEvidence.ControlComponentId = targetAC
					err := createEvidence(client, updatedEvidence)
					if err != nil {
						fmt.Printf("Error creating evidence for asset %s, ac %s: %v\n", assetID, targetAC, err)
						migrationResults = append(migrationResults, result)
						continue
					}
					evidenceCreated++
					result.Succeed = true
					migrationResults = append(migrationResults, result)
					fmt.Printf("Evidence created for asset %s, ac %s\n", assetID, targetAC)
				}
			}
		}
	}
	fmt.Printf("Total evidences created: %d, total required: %d\n", evidenceCreated, requiredCreate)
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
	fmt.Println("mapping data is valid")
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
	time.Sleep(1 * time.Second)
	fmt.Printf("Creating evidence for asset %s, ac %s\n", evidence.AssetId, evidence.ControlComponentId)
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

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
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

func exportMigrationDataToExcel(filePath string) error {
	f := excelize.NewFile()
	sheet := f.GetSheetName(f.GetActiveSheetIndex())

	headers := []string{"Asset ID", "Source ac", "Target AC", "Succeed"}
	for col, header := range headers {
		cellLocation, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sheet, cellLocation, header)
	}

	i := 2
	for _, r := range migrationResults {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", i), r.AssetID)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", i), r.SourceACID)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", i), r.TargetACID)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", i), r.Succeed)
		i++
	}

	if err := f.SaveAs(filePath); err != nil {
		return fmt.Errorf("failed to save Excel file: %v", err)
	}
	return nil
}
