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
│    • Open file with appropriate parser             │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 5. Process Records in Batches (2000)               │
│    For each record:                                │
│    • Parse into domain model                       │
│    • Validate (required fields, natural keys)      │
│    • Normalize (generate ID if missing)            │
│    • Add to batch                                  │
│    When batch full:                                │
│    • Upsert with OnConflict by natural key         │
│    • Update processed_count                        │
│    • Clear batch for next iteration                │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 6. Cleanup Orphaned Records                        │
│    • Find articles with invalid author_id          │
│    • Find comments with invalid article_id/user_id │
│    • Delete orphaned records                       │
│    • Log FK violations as errors                   │
└────────────┬───────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────┐
│ 7. Complete Processing                             │
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
    tags TEXT,                    -- JSON array of tags (denormalized)
    published_at DATETIME,
    created_at DATETIME,
    updated_at DATETIME,
    FOREIGN KEY (author_id) REFERENCES users(id)
);

-- Comments
CREATE TABLE comments (
    id TEXT PRIMARY KEY,
    article_id INTEGER NOT NULL,
    user_id TEXT NOT NULL,        -- FK to users table
    body TEXT NOT NULL,
    created_at DATETIME,
    updated_at DATETIME,
    FOREIGN KEY (article_id) REFERENCES articles(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Note: Tags are stored as JSON in articles.tags field
-- No separate tags or article_tags tables for performance
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

### Natural Key Upsert

The system uses natural keys for upsert operations to handle duplicate records efficiently:

```go
// Users: Upsert by email (natural key)
db.Clauses(clause.OnConflict{
    Columns:   []clause.Column{{Name: "email"}},
    DoUpdates: clause.AssignmentColumns(...),
}).Create(&validUsers)

// Articles: Upsert by slug (natural key)
db.Clauses(clause.OnConflict{
    Columns:   []clause.Column{{Name: "slug"}},
    DoUpdates: clause.AssignmentColumns(...),
}).Create(&validArticles)

// Comments: Upsert by ID (no natural key exists)
db.Clauses(clause.OnConflict{
    Columns:   []clause.Column{{Name: "id"}},
    DoUpdates: clause.AssignmentColumns(...),
}).Create(&validComments)
```

### Orphaned Record Cleanup

After bulk import, the system identifies and removes records with invalid foreign keys:

```go
// Find articles with non-existent author_id
db.Exec(`DELETE FROM articles WHERE id IN (
    SELECT a.id FROM articles a 
    LEFT JOIN users u ON a.author_id = u.id 
    WHERE u.id IS NULL
)`)

// Find comments with invalid FKs
db.Exec(`DELETE FROM comments WHERE id IN (
    SELECT c.id FROM comments c 
    LEFT JOIN articles a ON c.article_id = a.id 
    WHERE a.id IS NULL
)`)
```

### Validation Layers

1. **Parser Level**
   - File format correctness
   - Required CSV headers
   - Valid JSON structure

2. **Model Level**
   - Natural key presence (email for users, slug for articles)
   - Required fields per spec
   - ID generation if not provided

3. **Business Logic Level**
   - Email format validation
   - Slug kebab-case format validation
   - Comment body ≤ 500 words
   - Draft constraint (no published_at if status=draft)

4. **Database Level**
   - Upsert by natural key handles duplicates
   - Orphaned record cleanup for invalid FKs
   - Post-import validation and error logging

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
const batchSize = 2000

batch := make([]Article, 0, batchSize)
for record := range records {
    batch = append(batch, record)
    if len(batch) >= batchSize {
        // Upsert batch with natural key conflict resolution
        db.Clauses(clause.OnConflict{...}).Create(&batch)
        batch = batch[:0]   // Clear batch
    }
}
```

**Impact**: 100x faster than individual inserts, batch size optimized for performance

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

### 3. Natural Key Upserts

```go
// Upsert by email - duplicates update existing records
db.Clauses(clause.OnConflict{
    Columns:   []clause.Column{{Name: "email"}},
    DoUpdates: clause.AssignmentColumns([]string{
        "username", "bio", "image", "password", "updated_at",
    }),
}).Create(&validUsers)
```

**Impact**: Handles duplicate natural keys efficiently, no pre-loading needed

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

---

**Last Updated**: January 2026  
**Version**: 1.0.0
