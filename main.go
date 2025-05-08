package main

import (
	"fmt"
	"github.com/xuri/excelize/v2"
)

func main() {
	mapping, err := getDataFromCsv()
	if err != nil {
		fmt.Println("Error reading CSV file:", err)
		return
	}
	fmt.Println(mapping)
}

type MigrationMapData struct {
	SourceControlID string
	SourceACID      string
	TargetControlID string
	TargetACID      string
}

func getDataFromCsv() ([]MigrationMapData, error) {
	filePath := "mapping.xlsx"
	sheetName := "Sheet1"
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
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

	return results, nil
}
