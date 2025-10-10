package gcp

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Direct interface from https://pkg.go.dev/google.golang.org/api/sheets/v4#SpreadsheetsValuesService.

// This function retrieves data from a Google Sheet.
// Parameters:
// - spreadsheetId: the ID of the Google Sheet.
// - sheetName: the name of the sheet to retrieve data from.
// - cellRange: the range of cells to retrieve data from.
// Returns:
// - [][]interface{}: a 2D slice of interface{} values representing the retrieved data.
// - error: an error if one occurred, otherwise nil.
func (g *Gcp) SpreadsheetGet(spreadsheetId string, sheetName string, cellRange string) ([][]interface{}, error) {
	g.sheetClient()

	res, err := g.sheet.Spreadsheets.Values.Get(spreadsheetId, fmt.Sprintf("%s!%s", sheetName, cellRange)).Do()
	if err != nil || res.HTTPStatusCode != 200 {
		return nil, fmt.Errorf("unable to get data from range %s in sheet %s  <%v>", cellRange, sheetName, err)
	}

	if len(res.Values) == 0 {
		return nil, fmt.Errorf("no data found in range %s on sheet %s", cellRange, sheetName)
	}

	return res.Values, nil
}

// Appends a row of data to a Google Sheet.
// Parameters:
// - spreadsheetId: the ID of the Google Sheet.
// - sheetName: the name of the sheet to append data to.
// - valueRange: a slice of interface{} values representing the data to append.
// Returns:
// - string: an empty string.
// - error: an error if one occurred, otherwise nil.
func (g *Gcp) SpreadsheetAppend(spreadsheetId string, sheetName string, valueRange []interface{}) (string, error) {
	ctx := context.Background()
	g.sheetClient()

	row := &sheets.ValueRange{
		Values: [][]interface{}{valueRange},
	}

	res, err := g.sheet.Spreadsheets.Values.Append(spreadsheetId, sheetName, row).ValueInputOption("RAW").InsertDataOption("INSERT_ROWS").Context(ctx).Do()
	if err != nil || res.HTTPStatusCode != 200 {
		return "", fmt.Errorf("unable to append data into sheet %s <%v>", sheetName, err)
	}

	return "", nil
}

// Updates a range of cells in a Google Sheet.
// Parameters:
// - spreadsheetId: the ID of the Google Sheet.
// - sheetName: the name of the sheet to update data in.
// - cellRange: the range of cells to update data in.
// - valueRange: a slice of interface{} values representing the data to update.
// Returns:
// - string: an empty string.
// - error: an error if one occurred, otherwise nil.
func (g *Gcp) SpreadsheetUpdate(spreadsheetId string, sheetName string, cellRange string, valueRange []interface{}) (string, error) {
	ctx := context.Background()
	g.sheetClient()

	row := &sheets.ValueRange{
		Values: [][]interface{}{valueRange},
	}

	res, err := g.sheet.Spreadsheets.Values.Update(spreadsheetId, fmt.Sprintf("%s!%s", sheetName, cellRange), row).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil || res.HTTPStatusCode != 200 {
		return "", fmt.Errorf("unable to update data into sheet %s range %s <%v>", sheetName, cellRange, err)
	}

	return "", nil
}

// Similar to https://pkg.go.dev/google.golang.org/api/sheets/v4#SpreadsheetsService.GetByDataFilter
// Get a row from a Google Sheet based on filters.
// Parameters:
// - spreadsheetId: the ID of the Google Sheet.
// - sheetName: the name of the sheet to search data in.
// - filters: a map of column names to values to search for in the specified column.
// Returns:
// - map[string]interface{}: a map of the row data if a match is found.
// - error: an error if one occurred, otherwise nil.
func (g *Gcp) SpreadsheetGetRowByFilters(spreadsheetId string, sheetName string, filters map[string]string) (map[string]interface{}, error) {
	cellRange, headers, err := g.findCellRangeAndHeaders(spreadsheetId, sheetName)
	if err != nil {
		return nil, err
	}
	rows, _ := g.SpreadsheetGet(spreadsheetId, sheetName, cellRange)

	// Find matching rows based on the filters
	for _, row := range rows {
		match := true
		for key, value := range filters {
			headerIndex := findHeaderIndex(headers, key)
			if headerIndex == -1 || headerIndex >= len(row) || strings.TrimSpace(row[headerIndex].(string)) != value {
				match = false
				break
			}
		}
		if match {
			return mergeKV(headers, row), nil
		}
	}

	fmt.Printf("No row matches filters %v", filters)
	return nil, nil
}

// Creates a new sheet in a spreadsheet if it doesn't exist and adds header row.
// Parameters:
// - spreadsheetId: the ID of the Google Sheet.
// - sheetName: the name of the sheet to create.
// - headers: a slice of strings representing the column headers for the new sheet.
// Returns:
// - error: an error if one occurred, otherwise nil.
func (g *Gcp) SpreadsheetCreateIfNotExists(spreadsheetId string, sheetName string, headers []interface{}) error {
	ctx := context.Background()
	g.sheetClient()

	// Check if the spreadsheet exists
	spreadsheet, err := g.sheet.Spreadsheets.Get(spreadsheetId).Do()
	if err != nil {
		return fmt.Errorf("unable to get spreadsheet with ID %s: %v", spreadsheetId, err)
	}

	// Check if the sheet exists
	sheetExists := false
	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			sheetExists = true
			break
		}
	}

	// If sheet doesn't exist, create it
	if !sheetExists {
		// Create a new sheet
		addSheetRequest := &sheets.AddSheetRequest{
			Properties: &sheets.SheetProperties{
				Title: sheetName,
			},
		}

		// Create a batch update request
		batchUpdateRequest := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{
				{
					AddSheet: addSheetRequest,
				},
			},
		}

		// Execute the batch update
		_, err = g.sheet.Spreadsheets.BatchUpdate(spreadsheetId, batchUpdateRequest).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("unable to create sheet %s: %v", sheetName, err)
		}

		// Add headers if provided. Sort headers alphabetically for deterministic ordering
		// but keep "id" as the first column if it is present.
		if len(headers) > 0 {
			// Convert headers to []string
			strs := make([]string, 0, len(headers))
			hasID := false
			for _, h := range headers {
				s := fmt.Sprint(h)
				if s == "id" {
					hasID = true
					continue
				}
				strs = append(strs, s)
			}

			sort.Strings(strs)

			ordered := make([]interface{}, 0, len(headers))
			if hasID {
				ordered = append(ordered, "id")
			}
			for _, s := range strs {
				ordered = append(ordered, s)
			}

			headerRange := &sheets.ValueRange{
				Values: [][]interface{}{ordered},
			}

			_, err = g.sheet.Spreadsheets.Values.Update(spreadsheetId, fmt.Sprintf("%s!A1:%s1", sheetName, columnIndexToLetter(len(ordered)-1)), headerRange).ValueInputOption("RAW").Context(ctx).Do()
			if err != nil {
				return fmt.Errorf("unable to add headers to sheet %s: %v", sheetName, err)
			}
		}
	}

	return nil
}

// This function appends a row of data to a Google Sheet with a unique ID.
// Parameters:
// - spreadsheetId: the ID of the Google Sheet.
// - sheetName: the name of the sheet to append data to.
// - values: a slice of interface{} values representing the data to append.
// Returns:
// - string: the unique ID of the appended row.
// - error: an error if one occurred, otherwise nil.
func (g *Gcp) SpreadsheetAppendWithUniqueId(spreadsheetId string, sheetName string, values map[string]interface{}) (int64, error) {
	ctx := context.Background()
	g.sheetClient()

	// Extract keys from values map to create headers.
	// Build a deterministic, alphabetically-sorted list with "id" first.
	keyNames := make([]string, 0, len(values))
	for key := range values {
		if key == "id" {
			continue
		}
		keyNames = append(keyNames, key)
	}
	sort.Strings(keyNames)

	defaultHeaders := make([]interface{}, 0, len(keyNames)+1)
	defaultHeaders = append(defaultHeaders, "id")
	for _, k := range keyNames {
		defaultHeaders = append(defaultHeaders, k)
	}

	// Create sheet if it doesn't exist
	err := g.SpreadsheetCreateIfNotExists(spreadsheetId, sheetName, defaultHeaders)
	if err != nil {
		return 0, err
	}

	_, headers, err := g.findCellRangeAndHeaders(spreadsheetId, sheetName)
	if err != nil {
		return 0, err
	}

	rows, _ := g.SpreadsheetGet(spreadsheetId, sheetName, "A:A")
	id := getUniqueId(rows)
	values["id"] = id

	row := &sheets.ValueRange{
		Values: [][]interface{}{sortValuesByHeaders(headers, values)},
	}

	res, err := g.sheet.Spreadsheets.Values.Append(spreadsheetId, sheetName, row).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil || res.HTTPStatusCode != 200 {
		return 0, fmt.Errorf("unable to append data into sheet %s <%v>", sheetName, err)
	}

	return id, nil
}

func (g *Gcp) SpreadsheetGetUniqueIdByFiltersAndAppendIfNotExist(spreadsheetId string, sheetName string, filters map[string]string, values map[string]interface{}) (int64, error) {
	var id int64
	ctx := context.Background()
	g.sheetClient()

	// Build a deterministic, alphabetically-sorted list of headers from filters and values, with "id" first.
	keySet := make(map[string]struct{})
	for key := range filters {
		if key == "id" {
			continue
		}
		keySet[key] = struct{}{}
	}
	for key := range values {
		if key == "id" {
			continue
		}
		keySet[key] = struct{}{}
	}

	keyNames := make([]string, 0, len(keySet))
	for k := range keySet {
		keyNames = append(keyNames, k)
	}
	sort.Strings(keyNames)

	defaultHeaders := make([]interface{}, 0, len(keyNames)+1)
	defaultHeaders = append(defaultHeaders, "id")
	for _, k := range keyNames {
		defaultHeaders = append(defaultHeaders, k)
	}

	// Create the sheet if it doesn't exist with default headers
	err := g.SpreadsheetCreateIfNotExists(spreadsheetId, sheetName, defaultHeaders)
	if err != nil {
		return 0, err
	}

	_, headers, err := g.findCellRangeAndHeaders(spreadsheetId, sheetName)
	if err != nil {
		return 0, err
	}

	rowByFilters, _ := g.SpreadsheetGetRowByFilters(spreadsheetId, sheetName, filters)
	rows, _ := g.SpreadsheetGet(spreadsheetId, sheetName, "A:A")

	if rowByFilters == nil {
		id = getUniqueId(rows)
	} else {
		// fmt.Println(rowByFilters)
		idStr, ok := rowByFilters["id"].(string)
		if !ok {
			return 0, fmt.Errorf("unable to convert id to string")
		}

		i, err := strconv.ParseInt(idStr, 0, 64)
		if err != nil {
			return 0, fmt.Errorf("unable to parse string to int64 for %s: %v", idStr, err)
		}
		return i, nil
	}

	values["id"] = id

	row := &sheets.ValueRange{
		Values: [][]interface{}{sortValuesByHeaders(headers, values)},
	}

	res, err := g.sheet.Spreadsheets.Values.Append(spreadsheetId, sheetName, row).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil || res.HTTPStatusCode != 200 {
		log.Fatalf("unable to append data into sheet %s <%v>.", sheetName, err)
	}

	return id, nil
}

// This function initializes the Google Sheets client.
func (g *Gcp) sheetClient() {
	if g.sheet == nil {
		ctx := context.Background()
		jwt, err := getJwtConfig(g.keyByte, g.scope)
		if err != nil {
			log.Fatalf("could not get JWT config with scope %s <%v>.", g.scope, err)
		}

		c, err := sheets.NewService(ctx, option.WithTokenSource(jwt.TokenSource(ctx)))
		if err != nil {
			log.Fatalf("could not initialize Sheets client <%v>.", err)
		}

		g.sheet = c
	}
}

// This function returns the cell range of the first row of a Google Sheet.
// Parameters:
// - spreadsheetId: the ID of the Google Sheet.
// - sheetName: the name of the sheet to retrieve data from.
// Returns:
// - string: the cell range of the first row.
func (g *Gcp) findCellRangeAndHeaders(spreadsheetId string, sheetName string) (string, []interface{}, error) {
	rows, err := g.SpreadsheetGet(spreadsheetId, sheetName, "1:1")
	if err != nil {
		return "", nil, err
	}

	if len(rows) < 1 {
		return "", nil, fmt.Errorf("no headers found on sheet %s!%s", spreadsheetId, sheetName)
	}

	return fmt.Sprintf("A:%s", columnIndexToLetter(len(rows[0])-1)), rows[0], nil
}
