# Bulk Import/Export System

A high-performance bulk import and export system for the RealWorld Conduit API, built with Go and Gin framework. This system provides efficient data migration capabilities for users, articles, and comments with support for both CSV and NDJSON formats.

## 🚀 Features

- **Bulk Import**: Asynchronous import of large datasets (10K+ records)
- **Streaming Export**: Real-time streaming of data with minimal memory footprint
- **Async Export**: Background export jobs with downloadable files
- **Multiple Formats**: Support for CSV and NDJSON
- **Idempotency**: Prevent duplicate operations with idempotency keys
- **Filtering**: Export filtered data (e.g., published articles, active users)
- **Progress Tracking**: Real-time monitoring of import/export jobs
- **Metrics Collection**: API performance tracking with request IDs
- **Interactive Documentation**: Swagger/OpenAPI documentation at `/swagger/index.html`

## 📋 Prerequisites

- **Go**: 1.21 or higher
- **SQLite3**: For database operations
- **Git**: For version control

## 🔧 Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd bulk-import-export
```

2. Install dependencies:
```bash
go mod download
```

3. Build the application:
```bash
go build
```

## 🏃 Quick Start

1. Start the server:
```bash
./bulk-import-export
```

The server will start on `http://localhost:8080`

2. Access the API documentation:
```
http://localhost:8080/swagger/index.html
```

3. Import sample data:
```bash
curl -X POST http://localhost:8080/v1/imports \
  -H "Idempotency-Key: sample-users-001" \
  -F "file=@import_testdata/users_huge.csv" \
  -F "resource_type=users"
```

4. Check import status:
```bash
curl http://localhost:8080/v1/imports/{job_id}
```

5. Export data:
```bash
# Streaming export (immediate)
curl "http://localhost:8080/v1/exports?resource=users&format=csv" > users.csv

# Async export with filters
curl -X POST http://localhost:8080/v1/exports \
  -H "Content-Type: application/json" \
  -d '{
    "idempotency_key": "export-published-001",
    "resource_type": "articles",
    "format": "ndjson",
    "filters": {"status": "published"}
  }'
```

## 📁 Project Structure

```
.
├── hello.go                 # Main entry point with routes
├── common/                  # Shared utilities
│   ├── database.go         # Database connection
│   ├── job_models.go       # Import/Export job models
│   ├── metrics.go          # Metrics middleware
│   └── validation.go       # Validation utilities
├── imports/                 # Import module
│   ├── routers.go          # Import endpoints
│   └── doc.go              # Package documentation
├── exports/                 # Export module
│   ├── routers.go          # Export endpoints
│   └── doc.go              # Package documentation
├── parsers/                 # File format parsers
│   ├── csv.go              # CSV parser
│   ├── ndjson.go           # NDJSON parser
│   └── parsers_test.go     # Parser tests
├── users/                   # User domain
│   ├── models.go           # User models
│   └── validators.go       # User validation
├── articles/                # Article domain
│   ├── models.go           # Article models
│   └── validators.go       # Article validation
├── docs/                    # Swagger documentation (auto-generated)
├── data/                    # Runtime data
│   ├── gorm.db             # SQLite database
│   ├── uploads/            # Uploaded import files
│   └── exports/            # Generated export files
├── import_testdata/         # Large test datasets
└── test-suite.sh           # Comprehensive test automation
```

## 🧪 Testing

### Run Unit Tests
```bash
go test ./...
```

### Run Integration Tests
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run Comprehensive Test Suite
```bash
./test-suite.sh
```

The test suite includes:
- Database setup and cleanup
- Import 10K users, 15K articles, 20K comments
- Streaming export validation
- Async export with filters
- Idempotency verification
- Metrics collection verification

See [TEST_SUITE.md](TEST_SUITE.md) for details.

## 🔌 API Endpoints

### Imports
- `POST /v1/imports` - Create import job
- `GET /v1/imports/:job_id` - Get import status

### Exports
- `GET /v1/exports` - Streaming export (synchronous)
- `POST /v1/exports` - Create async export job
- `GET /v1/exports/:job_id` - Get export status

### Documentation
- `GET /swagger/*` - Interactive API documentation

For detailed endpoint documentation, see the [Swagger UI](http://localhost:8080/swagger/index.html) or [ARCHITECTURE.md](ARCHITECTURE.md).

## 🔑 Idempotency

All import and export operations require idempotency keys to prevent duplicate operations:

**Imports**: Use `Idempotency-Key` header
```bash
curl -H "Idempotency-Key: unique-key-123" ...
```

**Exports**: Include in JSON body
```json
{
  "idempotency_key": "unique-key-456",
  ...
}
```

Duplicate requests return the existing job (HTTP 200) instead of creating new ones (HTTP 202).

## 📈 Metrics

The system tracks API performance metrics including:
- Request ID (`X-Request-ID` header)
- Endpoint and HTTP method
- Response status code
- Duration in milliseconds
- Records processed
- Error counts
- Timestamp

Query metrics:
```bash
sqlite3 data/gorm.db "SELECT * FROM api_metrics ORDER BY timestamp DESC LIMIT 10"
```

## 🐛 Troubleshooting

### Import stuck at "processing"
Check server logs: `cat /tmp/server.log | tail -50`

### Database locked errors
Wait a moment for writes to complete, or use retry logic.

### File format errors
Ensure CSV files have headers and NDJSON files have one JSON object per line.

### Validation failures
Check error details in the import job response. Common issues:
- Missing required fields (email, slug, ID)
- Invalid email format
- Invalid slug format (must be kebab-case)
- Comment body exceeds 500 words
- Draft articles with published_at timestamp
- Invalid foreign key references (orphaned records cleaned up post-import)

Note: Duplicate natural keys (email, slug) are **not** failures - they update existing records via upsert.

## 🏗️ Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed system design, including:
- Data flow diagrams
- Validation strategy
- Database schema
- Performance optimizations
- Scalability considerations

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

Built with Go and Gin framework
