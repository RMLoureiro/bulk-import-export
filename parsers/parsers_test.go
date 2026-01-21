package parsers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCSV_ValidData(t *testing.T) {
	csvData := `id,email,name,role,active
user1,test@example.com,Test User,admin,true
user2,test2@example.com,Test User 2,reader,false`

	reader := strings.NewReader(csvData)
	records, errors := ParseCSV(reader)

	// Collect all records
	var allRecords []Record
	for record := range records {
		allRecords = append(allRecords, record)
	}

	// Check for errors
	var allErrors []error
	for err := range errors {
		allErrors = append(allErrors, err)
	}

	assert.Len(t, allRecords, 2, "Should parse 2 records")
	assert.Len(t, allErrors, 0, "Should have no errors")

	// Verify first record
	assert.Equal(t, "user1", allRecords[0]["id"])
	assert.Equal(t, "test@example.com", allRecords[0]["email"])
	assert.Equal(t, "Test User", allRecords[0]["name"])
	assert.Equal(t, "admin", allRecords[0]["role"])
	assert.Equal(t, "true", allRecords[0]["active"])

	// Verify second record
	assert.Equal(t, "user2", allRecords[1]["id"])
	assert.Equal(t, "false", allRecords[1]["active"])
}

func TestParseCSV_EmptyFile(t *testing.T) {
	csvData := ``
	reader := strings.NewReader(csvData)
	records, errors := ParseCSV(reader)

	var allRecords []Record
	for record := range records {
		allRecords = append(allRecords, record)
	}

	var allErrors []error
	for err := range errors {
		allErrors = append(allErrors, err)
	}

	assert.Len(t, allRecords, 0, "Should parse 0 records")
	assert.Len(t, allErrors, 0, "Empty file should not error")
}

func TestParseCSV_MissingValues(t *testing.T) {
	csvData := `id,email,name
user1,test@example.com
user2,test2@example.com,User 2`

	reader := strings.NewReader(csvData)
	records, errors := ParseCSV(reader)

	var allRecords []Record
	for record := range records {
		allRecords = append(allRecords, record)
	}

	// Drain errors
	for range errors {
	}

	assert.Len(t, allRecords, 2)
	// First record has missing "name" column
	assert.Equal(t, "", allRecords[0]["name"], "Missing value should be empty string")
	assert.Equal(t, "User 2", allRecords[1]["name"])
}

func TestParseCSV_WithCommasInValues(t *testing.T) {
	csvData := `id,name,description
1,"Smith, John","A person with comma in name"
2,Jane,"Description, with, commas"`

	reader := strings.NewReader(csvData)
	records, errors := ParseCSV(reader)

	var allRecords []Record
	for record := range records {
		allRecords = append(allRecords, record)
	}

	// Drain errors
	for range errors {
	}

	assert.Len(t, allRecords, 2)
	assert.Equal(t, "Smith, John", allRecords[0]["name"])
	assert.Equal(t, "A person with comma in name", allRecords[0]["description"])
}

func TestParseNDJSON_ValidData(t *testing.T) {
	ndjsonData := `{"id":"article1","slug":"test-article","title":"Test Article","status":"published"}
{"id":"article2","slug":"draft-article","title":"Draft Article","status":"draft"}`

	reader := strings.NewReader(ndjsonData)
	records, errors := ParseNDJSON(reader)

	// Collect all records
	var allRecords []map[string]interface{}
	for record := range records {
		allRecords = append(allRecords, record)
	}

	// Check for errors
	var allErrors []error
	for err := range errors {
		allErrors = append(allErrors, err)
	}

	assert.Len(t, allRecords, 2, "Should parse 2 records")
	assert.Len(t, allErrors, 0, "Should have no errors")

	// Verify first record
	assert.Equal(t, "article1", allRecords[0]["id"])
	assert.Equal(t, "test-article", allRecords[0]["slug"])
	assert.Equal(t, "published", allRecords[0]["status"])

	// Verify second record
	assert.Equal(t, "draft", allRecords[1]["status"])
}

func TestParseNDJSON_EmptyLines(t *testing.T) {
	ndjsonData := `{"id":"article1","title":"First"}

{"id":"article2","title":"Second"}
`

	reader := strings.NewReader(ndjsonData)
	records, errors := ParseNDJSON(reader)

	var allRecords []map[string]interface{}
	for record := range records {
		allRecords = append(allRecords, record)
	}

	// Drain errors
	for range errors {
	}

	assert.Len(t, allRecords, 2, "Should skip empty lines")
}

func TestParseNDJSON_InvalidJSON(t *testing.T) {
	ndjsonData := `{"id":"article1","title":"Valid"}
{invalid json}
{"id":"article2","title":"Valid Again"}`

	reader := strings.NewReader(ndjsonData)
	records, errors := ParseNDJSON(reader)

	var allRecords []map[string]interface{}
	for record := range records {
		allRecords = append(allRecords, record)
	}

	var allErrors []error
	for err := range errors {
		allErrors = append(allErrors, err)
	}

	assert.Len(t, allRecords, 2, "Should parse valid records")
	assert.Len(t, allErrors, 1, "Should report 1 error for invalid JSON")
}

func TestParseNDJSON_NestedObjects(t *testing.T) {
	ndjsonData := `{"id":"1","author":{"id":"auth1","name":"John"},"tags":["go","test"]}
{"id":"2","metadata":{"count":5,"active":true}}`

	reader := strings.NewReader(ndjsonData)
	records, errors := ParseNDJSON(reader)

	var allRecords []map[string]interface{}
	for record := range records {
		allRecords = append(allRecords, record)
	}

	// Drain errors
	for range errors {
	}

	assert.Len(t, allRecords, 2)

	// Verify nested object
	author := allRecords[0]["author"].(map[string]interface{})
	assert.Equal(t, "auth1", author["id"])
	assert.Equal(t, "John", author["name"])

	// Verify array
	tags := allRecords[0]["tags"].([]interface{})
	assert.Len(t, tags, 2)
	assert.Equal(t, "go", tags[0])
}

func TestParseNDJSON_EmptyFile(t *testing.T) {
	ndjsonData := ``
	reader := strings.NewReader(ndjsonData)
	records, errors := ParseNDJSON(reader)

	var allRecords []map[string]interface{}
	for record := range records {
		allRecords = append(allRecords, record)
	}

	var allErrors []error
	for err := range errors {
		allErrors = append(allErrors, err)
	}

	assert.Len(t, allRecords, 0)
	assert.Len(t, allErrors, 0)
}
