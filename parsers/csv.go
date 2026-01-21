package parsers

import (
	"encoding/csv"
	"io"
)

// Record represents a single CSV row as a map of column name to value
type Record map[string]string

// ParseCSV reads CSV from io.Reader and streams records via channel
// Returns two channels: one for records, one for errors
// Caller must consume both channels to avoid goroutine leak
func ParseCSV(reader io.Reader) (<-chan Record, <-chan error) {
	records := make(chan Record, 100) // Buffered for better throughput
	errors := make(chan error, 1)
	
	go func() {
		defer close(records)
		defer close(errors)
		
		csvReader := csv.NewReader(reader)
		csvReader.ReuseRecord = true // Reuse slice for better performance
		csvReader.FieldsPerRecord = -1 // Allow variable number of fields
		
		// Read header row
		headers, err := csvReader.Read()
		if err != nil {
			if err != io.EOF {
				errors <- err
			}
			return
		}
		
		// Make a copy of headers since we're reusing the record slice
		headersCopy := make([]string, len(headers))
		copy(headersCopy, headers)
		
		// Read data rows
		for {
			row, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				errors <- err
				continue // Skip malformed rows, continue processing
			}
			
			// Map row to headers
			record := make(Record)
			for i, header := range headersCopy {
				if i < len(row) {
					record[header] = row[i]
				} else {
					record[header] = "" // Missing column value
				}
			}
			
			records <- record
		}
	}()
	
	return records, errors
}
