package parsers

import (
	"bufio"
	"encoding/json"
	"io"
)

// ParseNDJSON reads NDJSON (newline-delimited JSON) from io.Reader and streams records via channel
// Each line should be a valid JSON object
// Returns two channels: one for records, one for errors
// Caller must consume both channels to avoid goroutine leak
func ParseNDJSON(reader io.Reader) (<-chan map[string]interface{}, <-chan error) {
	records := make(chan map[string]interface{}, 100) // Buffered for better throughput
	errors := make(chan error, 1)
	
	go func() {
		defer close(records)
		defer close(errors)
		
		scanner := bufio.NewScanner(reader)
		
		// Increase buffer size for large lines (up to 1MB per line)
		const maxCapacity = 1024 * 1024 // 1MB
		buf := make([]byte, maxCapacity)
		scanner.Buffer(buf, maxCapacity)
		
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Bytes()
			
			// Skip empty lines
			if len(line) == 0 {
				continue
			}
			
			var record map[string]interface{}
			if err := json.Unmarshal(line, &record); err != nil {
				// Send error but continue processing
				errors <- err
				continue
			}
			
			records <- record
		}
		
		// Check for scanner errors (e.g., line too long)
		if err := scanner.Err(); err != nil {
			errors <- err
		}
	}()
	
	return records, errors
}
