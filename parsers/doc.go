// Package parsers provides streaming parsers for CSV and NDJSON file formats.
//
// The parsers are designed for memory-efficient processing of large files by streaming
// records through Go channels, avoiding the need to load entire files into memory.
//
// Both parsers return two channels:
//   - A records channel that streams parsed data
//   - An errors channel for non-fatal parsing errors
//
// Callers must consume both channels to avoid goroutine leaks.
//
// Example usage for CSV:
//
//	file, _ := os.Open("data.csv")
//	defer file.Close()
//	records, errors := parsers.ParseCSV(file)
//
//	go func() {
//	    for err := range errors {
//	        log.Printf("CSV error: %v", err)
//	    }
//	}()
//
//	for record := range records {
//	    // Process each record (map[string]string)
//	    fmt.Println(record["email"])
//	}
//
// Example usage for NDJSON:
//
//	file, _ := os.Open("data.ndjson")
//	defer file.Close()
//	records, errors := parsers.ParseNDJSON(file)
//
//	go func() {
//	    for err := range errors {
//	        log.Printf("NDJSON error: %v", err)
//	    }
//	}()
//
//	for record := range records {
//	    // Process each record (map[string]interface{})
//	    fmt.Println(record["slug"])
//	}
package parsers
