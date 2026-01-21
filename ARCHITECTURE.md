# System Architecture

This document describes the architecture, design decisions, and implementation details of the Bulk Import/Export System.

## Table of Contents

1. [Overview](#overview)
2. [System Design](#system-design)
3. [Data Flow](#data-flow)
4. [Database Schema](#database-schema)
5. [Validation Strategy](#validation-strategy)
6. [Performance Optimizations](#performance-optimizations)
7. [Scalability Considerations](#scalability-considerations)
8. [API Design](#api-design)

## Overview

The system is built on the principle of **separation of concerns** with distinct modules for imports, exports, parsing, and validation. It uses asynchronous processing for imports and provides both streaming and async export capabilities.

### Key Design Goals

1. **Performance**: Handle 10K+ records efficiently
2. **Memory Efficiency**: O(1) memory usage via streaming
3. **Reliability**: Idempotency prevents duplicate operations
4. **Observability**: Comprehensive metrics and progress tracking
5. **Maintainability**: Clean separation of concerns

## System Design

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                       Client Layer                          │
│  (curl, Postman, Frontend, API consumers)                  │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                    API Gateway (Gin)                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Metrics    │  │   Recovery   │  │    Logger    │     │
│  │  Middleware  │  │  Middleware  │  │  Middleware  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────┬──────────────────────────────────────┬────────────────┘
      │                                       │
      ▼                                       ▼
┌──────────────────────────┐    ┌─────────────────────────────┐
│    Import Endpoints      │    │     Export Endpoints        │
│  POST /v1/imports        │    │  GET  /v1/exports (stream)  │
│  GET  /v1/imports/:id    │    │  POST /v1/exports (async)   │
│                          │    │  GET  /v1/exports/:id       │
└───────────┬──────────────┘    └─────────────┬───────────────┘
            │                                   │
            ▼                                   ▼
┌────────────────────────────────────────────────────────────┐
│                   Business Logic Layer                      │
│                                                              │
│  ┌──────────────┐    ┌────────────────┐    ┌────────────┐ │
│  │   Import     │    │     Export     │    │  Parser    │ │
│  │  Processor   │    │   Processor    │    │  Manager   │ │
│  └───┬──────────┘    └────────┬───────┘    └─────┬──────┘ │
│      │                         │                   │        │
│      │   ┌─────────────────────┴───────────────────┘        │
│      │   │                                                  │
│      ▼   ▼                                                  │
│  ┌────────────────────────────────────────────┐           │
│  │         Validation Layer                    │           │
│  │  • Pre-load reference data into maps       │           │
│  │  • O(1) lookup for foreign keys            │           │
│  │  • Business rule validation                 │           │
│  └────────────────────────────────────────────┘           │
└────────────────────┬───────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                   Data Access Layer (GORM)                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │    Users     │  │   Articles   │  │   Comments   │     │
│  │    Table     │  │    Table     │  │    Table     │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ Import Jobs  │  │ Export Jobs  │  │ API Metrics  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
                     │
                     ▼
           ┌──────────────────┐
           │  SQLite Database  │
           │   (gorm.db)       │
           └──────────────────┘
```

### Module Breakdown

#### 1. **hello.go** - Main Application
- Initializes database connection
- Registers middleware (Metrics, Recovery, Logger)
- Defines API routes
- Serves Swagger documentation

#### 2. **common/** - Shared Utilities
- `database.go`: Database connection pool management
- `job_models.go`: ImportJob and ExportJob models
- `metrics.go`: API metrics middleware
- `validation.go`: Shared validation utilities

#### 3. **imports/** - Import Module
- `routers.go`: Import endpoint handlers
- Async job processing with goroutines
- Idempotency checks
- Progress tracking

#### 4. **exports/** - Export Module
- `routers.go`: Export endpoint handlers
- Streaming exports (synchronous)
- Async exports with downloadable files
- Filter support

#### 5. **parsers/** - File Format Parsers
- `csv.go`: CSV parsing with header detection
- `ndjson.go`: Newline-delimited JSON parsing
- Streaming parsers for memory efficiency

## Data Flow

### Import Flow

```
Client Upload
     │
     ▼
┌────────────────────────────────────────────────────┐
│ 1. Receive File (multipart) or URL (JSON)         │
│    • Validate Idempotency-Key header               │
│    • Check for existing job                        │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 2. Save File to ./data/uploads/                    │
│    • Generate UUID filename                        │
│    • Determine format (CSV/NDJSON)                 │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 3. Create ImportJob Record                         │
│    • Status: "pending"                             │
│    • Store idempotency_key, resource_type, format  │
│    • Return 202 Accepted with job_id               │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 4. Start Background Processing (goroutine)         │
│    • Status → "processing"                         │
│    • Pre-load reference data (users for articles)  │
│    • Open file with appropriate parser             │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 5. Process Records in Batches (1000)               │
│    For each record:                                │
│    • Parse into domain model                       │
│    • Validate (required fields, foreign keys)      │
│    • Add to batch                                  │
│    When batch full:                                │
│    • Bulk insert with db.Create(&batch)            │
│    • Update processed_count                        │
│    • Clear batch for next iteration                │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 6. Complete Processing                             │
│    • Status → "completed" or "failed"              │
│    • Set completed_at timestamp                    │
│    • Record success_count and fail_count           │
│    • Store validation errors                       │
└────────────────────────────────────────────────────┘
```

### Export Flow (Streaming)

```
Client Request (GET /v1/exports?resource=users&format=csv)
     │
     ▼
┌────────────────────────────────────────────────────┐
│ 1. Validate Parameters                             │
│    • resource: users, articles, comments           │
│    • format: csv, ndjson                           │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 2. Set Response Headers                            │
│    • Content-Type: text/csv or application/x-ndjson│
│    • Content-Disposition: attachment               │
│    • Transfer-Encoding: chunked                    │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 3. Stream Data (c.Stream)                          │
│    Loop in batches (1000):                         │
│    • Query records with LIMIT/OFFSET               │
│    • Serialize to CSV row or NDJSON line           │
│    • Write directly to response writer             │
│    • Flush writer (no accumulation)                │
│    Until: No more records                          │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 4. Close Stream                                    │
│    • Client receives complete file                 │
│    • Memory usage stays constant (O(1))            │
└────────────────────────────────────────────────────┘
```

### Export Flow (Async)

```
Client Request (POST /v1/exports with filters)
     │
     ▼
┌────────────────────────────────────────────────────┐
│ 1. Validate Request Body                           │
│    • idempotency_key (required)                    │
│    • resource_type, format                         │
│    • filters (optional)                            │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 2. Check Idempotency                               │
│    • Query existing jobs by idempotency_key        │
│    • If exists: Return 200 with existing job       │
│    • If new: Continue to step 3                    │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 3. Create ExportJob Record                         │
│    • Status: "pending"                             │
│    • Store filters JSON                            │
│    • Return 202 Accepted with job_id               │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 4. Start Background Processing (goroutine)         │
│    • Status → "processing"                         │
│    • Generate filename with UUID                   │
│    • Open file in ./data/exports/                  │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 5. Query and Write Filtered Data                   │
│    • Build query with filters (WHERE clauses)      │
│    • Process in batches (1000)                     │
│    • Write to file                                 │
│    • Update total_records count                    │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 6. Complete and Generate Download URL              │
│    • Status → "completed"                          │
│    • download_url: "/exports/{filename}"           │
│    • completed_at timestamp                        │
│    Client polls GET /v1/exports/:job_id            │
│    Downloads file from /exports/{filename}         │
└────────────────────────────────────────────────────┘
```

## Database Schema

### Domain Tables

```sql
-- Users
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    username TEXT UNIQUE NOT NULL,
    bio TEXT,
    image TEXT,
    password TEXT NOT NULL,
    created_at DATETIME,
    updated_at DATETIME
);

-- Articles
CREATE TABLE articles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT UNIQUE NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    body TEXT NOT NULL,
    author_id TEXT NOT NULL,
    status TEXT DEFAULT 'draft',
    created_at DATETIME,
    updated_at DATETIME,
    FOREIGN KEY (author_id) REFERENCES users(id)
);

-- Comments
CREATE TABLE comments (
    id TEXT PRIMARY KEY,          -- Format: cm_{article_slug}_{index}
    article_id INTEGER NOT NULL,
    author_id TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at DATETIME,
    updated_at DATETIME,
    FOREIGN KEY (article_id) REFERENCES articles(id),
    FOREIGN KEY (author_id) REFERENCES users(id)
);

-- Tags (many-to-many with articles)
CREATE TABLE tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag TEXT UNIQUE NOT NULL
);

CREATE TABLE article_tags (
    article_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (article_id, tag_id),
    FOREIGN KEY (article_id) REFERENCES articles(id),
    FOREIGN KEY (tag_id) REFERENCES tags(id)
);
```

### Job Tracking Tables

```sql
-- Import Jobs
CREATE TABLE import_jobs (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT UNIQUE NOT NULL,
    resource_type TEXT NOT NULL,
    format TEXT NOT NULL,
    file_path TEXT NOT NULL,
    status TEXT NOT NULL,
    total_records INTEGER DEFAULT 0,
    processed_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    fail_count INTEGER DEFAULT 0,
    errors TEXT,  -- JSON array of validation errors
    created_at DATETIME,
    updated_at DATETIME,
    completed_at DATETIME
);

-- Export Jobs
CREATE TABLE export_jobs (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT UNIQUE NOT NULL,
    resource_type TEXT NOT NULL,
    format TEXT NOT NULL,
    filters TEXT,  -- JSON object with filter criteria
    fields TEXT,   -- JSON array of field names
    status TEXT NOT NULL,
    total_records INTEGER DEFAULT 0,
    download_url TEXT,
    created_at DATETIME,
    updated_at DATETIME,
    completed_at DATETIME
);

-- API Metrics
CREATE TABLE api_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    method TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    rows_processed INTEGER DEFAULT 0,
    errors INTEGER DEFAULT 0,
    timestamp DATETIME
);
```

## Validation Strategy

### Pre-Loading Reference Data

Before importing articles or comments, the system pre-loads reference data into maps for O(1) lookup:

```go
// Example: Pre-load users for article validation
validUsers := make(map[string]bool)
var users []users.UserModel
db.Find(&users)
for _, user := range users {
    validUsers[user.ID] = true
}

// Later, during validation:
if !validUsers[article.AuthorID] {
    // Invalid author_id
}
```

### Validation Layers

1. **Parser Level**
   - File format correctness
   - Required CSV headers
   - Valid JSON structure

2. **Model Level**
   - Field presence (required fields)
   - Data types
   - Field lengths

3. **Business Logic Level**
   - Foreign key references (author_id exists in users)
   - Unique constraints (email, username, slug)
   - Business rules (email format, slug format)

4. **Database Level**
   - NOT NULL constraints
   - UNIQUE indexes
   - Foreign key constraints

### Error Handling

Validation errors are collected but don't stop processing:

```go
type RecordValidationResult struct {
    Line   int      `json:"line"`
    Errors []string `json:"errors"`
}

// Continue processing even if some records fail
// Store errors in job.Errors for later review
```

## Performance Optimizations

### 1. Batch Processing

```go
const batchSize = 1000

batch := make([]Article, 0, batchSize)
for record := range records {
    batch = append(batch, record)
    if len(batch) >= batchSize {
        db.Create(&batch)  // Single transaction for 1000 records
        batch = batch[:0]   // Clear batch
    }
}
```

**Impact**: 100x faster than individual inserts

### 2. Streaming I/O

```go
// Don't load entire file into memory
file, _ := os.Open(filename)
reader := csv.NewReader(file)

for {
    record, err := reader.Read()  // Read one line at a time
    if err == io.EOF {
        break
    }
    // Process record
}
```

**Impact**: O(1) memory usage regardless of file size

### 3. Pre-loaded Reference Maps

```go
validUsers := make(map[string]bool)
// Load once before processing
db.Find(&users)
for _, user := range users {
    validUsers[user.ID] = true
}

// O(1) lookups during processing
if !validUsers[article.AuthorID] {
    // Invalid
}
```

**Impact**: Eliminates N database queries during validation

### 4. Async Processing with Goroutines

```go
go ProcessImportJob(job.ID)  // Don't block HTTP response

// Client can poll status
GET /v1/imports/:job_id
```

**Impact**: Immediate API response, background processing

### 5. Connection Pooling

```go
sqlDB, _ := db.DB()
sqlDB.SetMaxOpenConns(25)
sqlDB.SetMaxIdleConns(25)
```

**Impact**: Reuse connections, reduce connection overhead

## Scalability Considerations

### Current Limitations

1. **Single Server**: No horizontal scaling
2. **SQLite**: Single-writer limitation
3. **File Storage**: Local disk only
4. **No Queue**: Simple goroutines for async jobs

### Scaling Path

#### Phase 1: Optimize Current Setup (10K-100K records)
- ✅ Batch processing
- ✅ Streaming I/O
- ✅ Connection pooling
- ✅ Async processing

#### Phase 2: Scale Vertically (100K-1M records)
- Migrate to PostgreSQL (multi-writer)
- Increase batch size (2000-5000)
- Add more database connections
- Optimize queries with indexes

#### Phase 3: Scale Horizontally (1M+ records)
- Add message queue (RabbitMQ, Redis)
- Multiple worker processes
- Load balancer for API
- S3/MinIO for file storage
- Distributed tracing (OpenTelemetry)

#### Phase 4: Enterprise Scale (10M+ records)
- Kubernetes for orchestration
- Apache Kafka for event streaming
- Distributed database (CockroachDB)
- Separate read replicas
- CDN for export downloads

### Estimated Capacity

| Component | Current | Phase 2 | Phase 3 | Phase 4 |
|-----------|---------|---------|---------|---------|
| Records/Import | 100K | 1M | 10M | 100M+ |
| Concurrent Jobs | 10 | 100 | 1000 | 10K+ |
| Import Speed | 5K/sec | 20K/sec | 100K/sec | 1M/sec |
| Storage | Local Disk | NAS | S3 | Distributed Object Store |

## API Design

### RESTful Principles

1. **Resource-Based URLs**: `/v1/imports/:job_id` not `/v1/getImport`
2. **HTTP Methods**: POST for create, GET for retrieve
3. **Status Codes**: 
   - 200 OK (idempotent duplicate)
   - 202 Accepted (async job created)
   - 400 Bad Request (validation error)
   - 404 Not Found (job not found)
   - 500 Internal Server Error

4. **Idempotency**: Required keys prevent duplicates
5. **Pagination**: (Future) via `offset` and `limit` query params

### Request/Response Patterns

#### Import Creation
```
POST /v1/imports
Headers:
  Idempotency-Key: unique-123
  Content-Type: multipart/form-data
Body:
  file: <binary>
  resource_type: users

Response 202:
{
  "job_id": "uuid-here",
  "status": "pending",
  "created_at": "2026-01-21T18:00:00Z"
}
```

#### Job Status Polling
```
GET /v1/imports/:job_id

Response 200:
{
  "job_id": "uuid-here",
  "resource_type": "users",
  "status": "completed",
  "total_records": 10000,
  "processed_count": 10000,
  "success_count": 9750,
  "fail_count": 250,
  "errors": [
    {"line": 42, "errors": ["invalid email format"]},
    {"line": 156, "errors": ["missing required field: username"]}
  ],
  "created_at": "2026-01-21T18:00:00Z",
  "completed_at": "2026-01-21T18:02:15Z"
}
```

### Swagger Documentation

The API is fully documented with OpenAPI 3.0 annotations. Access interactive docs at:

```
http://localhost:8080/swagger/index.html
```

Documentation includes:
- Request/response schemas
- Parameter descriptions
- Example requests
- HTTP status codes
- Authentication requirements (future)

## Security Considerations

### Current Implementation

1. **Idempotency Keys**: Prevent duplicate operations
2. **Input Validation**: Strict validation of all inputs
3. **File Upload Limits**: (Should add) Max file size
4. **Error Messages**: Don't expose internal details

### Future Security Enhancements

1. **Authentication**: JWT tokens for API access
2. **Authorization**: Role-based access control (RBAC)
3. **Rate Limiting**: Prevent abuse
4. **File Scanning**: Virus/malware detection
5. **Encryption**: TLS/SSL for data in transit
6. **Audit Logging**: Track all operations
7. **CORS**: Configure allowed origins

---

**Last Updated**: January 2026  
**Version**: 1.0.0
