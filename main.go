package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/xuri/excelize/v2"
	"io"
	"log"
	"net/http"
)

const (
	statusConnStr = "user=myuser password=mypassword dbname=controlstatus sslmode=disable"
	codexConnStr  = "user=myuser password=mypassword dbname=codex sslmode=disable"
	// baseURL    = "https://australia-southeast1-anz-x-xplore-np-4a74dd.cloudfunctions.net/xp-cf-xplore-api"
	baseURL      = "https://australia-southeast1-anz-x-xplore-prod-44f597.cloudfunctions.net/xp-cf-xplore-api"
	assetsPath   = "/api/v1/assets"
	gciPath      = "/api/v1/generic-control-instances/"
	evidencePath = "/api/v1/evidence"
	token        = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjA3YjgwYTM2NTQyODUyNWY4YmY3Y2QwODQ2ZDc0YThlZTRlZjM2MjUiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb20iLCJhenAiOiIzMjU1NTk0MDU1OS5hcHBzLmdvb2dsZXVzZXJjb250ZW50LmNvbSIsImF1ZCI6IjMyNTU1OTQwNTU5LmFwcHMuZ29vZ2xldXNlcmNvbnRlbnQuY29tIiwic3ViIjoiMTE2MTU0Nzk3ODg1ODQ0NDcyMzkxIiwiaGQiOiJhbnouY29tIiwiZW1haWwiOiJjZWxpbmUubWFAYW56LmNvbSIsImVtYWlsX3ZlcmlmaWVkIjp0cnVlLCJhdF9oYXNoIjoibXVOUlp5czZoTDN6dUNnaHI0ZUFTdyIsImlhdCI6MTc0NjcxMjIyMywiZXhwIjoxNzQ2NzE1ODIzfQ.L8a2qb1vksYLEHbe88TN_4keL7es1VddYAqkN2gWtMYWUlRDr6K1LkxdIS-PVG5Rp-uwqCAunRLLwAgKm2UpajY1v_VSccPjfLKYUsQFiKdL7fu7TceaOeO-hW3r9y-hGfkS4CFstV4O_LAsSUxpnwV7F7bWmIDze4UIS7nTKChr6761IfgBEhoLp7_kUDlxV1UKQewsaAMFe_doTegFMiOircepYABogvOkKb4KFje5CjOPa0uf3Wwf3TQ3u9LMGBNs5QUN_QwUWt7cpugzqHeOin3x5Tr45Jem4ehh_Ji1GX0MxqoBKXMASIvxdSlAR8QGPmWIlCe7znqfZrwqSg"
)

type MigrationMapData struct {
	SourceControlID string
	SourceACID      string
	TargetControlID string
	TargetACID      string
}

func main() {
	mapping := getDataFromCsv()

	processedMap := processMappingData(mapping)
	verifyMappingData(processedMap)

	assetIDs := readAssetIDs()

	migrate(assetIDs, processedMap)
}

func migrate(assetIDs []string, processedMap map[key][]key) {
	client := &http.Client{}
	for _, assetID := range assetIDs {
		for sourceKey, targetKeys := range processedMap {
			evidences := readEvidences(assetID, sourceKey.acID)
			for _, target := range targetKeys {
				for _, evidence := range evidences {
					updatedEvidence := evidence
					updatedEvidence.ControlId = target.controlID
					updatedEvidence.ControlComponentId = target.acID
					err := createEvidence(client, updatedEvidence)
					if err != nil {
						fmt.Printf("Error creating evidence for asset ID %s: %v\n", assetID, err)
					}
				}
			}
		}
	}
}

func getDataFromCsv() []MigrationMapData {
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

	var results []MigrationMapData
	for i, row := range rows {
		if i == 0 {
			continue
		}
		res := MigrationMapData{
			SourceControlID: row[0],
			SourceACID:      row[1],
			TargetControlID: row[2],
			TargetACID:      row[3],
		}
		results = append(results, res)
	}

	return results
}

func verifyMappingData(data map[key][]key) {
	// one evidence can be mapped to multiple tcl
	// not more than one tcl evidence should be created
	counter := make(map[key]int)
	for _, item := range data {
		for _, targetKey := range item {
			counter[targetKey]++
			if counter[targetKey] > 1 {
				panic(errors.New("duplicate target control and ac found"))
			}
		}
	}
	fmt.Println("mapping data is valid")
}

// func readEvidences() []controlsdb.Evidence {
// 	connStr := "user=myuser password=mypassword dbname=controlstatus sslmode=disable"
// 	db, err := sql.Open("postgres", connStr)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	defer db.Close()
//
// 	rows, err := db.Query(`select * from evidences;`)
// 	if err != nil {
// 		log.Fatalf("Query failed: %v", err)
// 	}
// 	defer rows.Close()
//
// 	var results []controlsdb.Evidence
//
// 	for rows.Next() {
// 		var result controlsdb.Evidence
// 		err := rows.Scan(&result)
// 		if err != nil {
// 			log.Printf("Failed to scan row: %v", err)
// 			continue
// 		}
// 		results = append(results, result)
// 	}
//
// 	return results
// }

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

func readAssetIDs() []string {
	db, err := sqlx.Open("postgres", codexConnStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Queryx("select asset_id from codex_assets where status != 'Retired' and status != 'Reassigned' order by asset_id;")
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
		result = append(result, row["asset_id"].(string))
	}
	return result
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

type key struct {
	controlID string
	acID      string
}

func processMappingData(mapping []MigrationMapData) map[key][]key {
	results := make(map[key][]key)
	for _, data := range mapping {
		sourceACID := data.SourceControlID + "." + data.SourceACID
		sourceKey := key{data.SourceControlID, sourceACID}
		targetACID := data.TargetControlID + "." + data.TargetACID
		targetKey := key{data.TargetControlID, targetACID}
		results[sourceKey] = append(results[sourceKey], targetKey)
	}
	fmt.Println("there are ", len(results), "mapping data")
	return results
}

func createEvidence(client *http.Client, evidence Evidence) error {
	fmt.Printf("creating evidence for asset %s, ac %s\n", evidence.AssetId, evidence.ControlComponentId)
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
	var responseBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

func sendPOSTRequest[T any](client *http.Client, url string, payload T) (*http.Response, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	fmt.Printf("jsonData is %v\n", string(jsonData))

	// Create the request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}
