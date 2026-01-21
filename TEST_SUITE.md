# Test Suite Documentation

## Overview
The `test-suite.sh` script provides comprehensive end-to-end testing of the bulk import/export system. It validates the complete workflow from database cleanup through imports, exports, and metrics collection.

## Running the Test Suite

```bash
./test-suite.sh
```

## What the Test Suite Does

### Step 1: Clean Database
- Stops any running server instances
- Removes the SQLite database file
- Clears upload and export directories
- Provides a clean state for testing

### Step 2: Start Server
- Rebuilds the application
- Starts server in background
- Verifies server is responding

### Step 3: Import Users
- Uploads `users_huge.csv` (10,000 user records)
- Monitors job status during processing
- Waits for completion
- **Verifications:**
  - ✓ Import completes successfully
  - ✓ Success count > 9,000 users
  - ✓ Database count matches success count

### Step 4: Import Articles
- Uploads `articles_huge.ndjson` (15,000 article records)
- Waits for completion
- **Verifications:**
  - ✓ Import completes successfully
  - ✓ Success count > 10,000 articles
  - ✓ Database count matches success count

### Step 5: Import Comments
- Uploads `comments_huge.ndjson` (20,000 comment records)
- Waits for completion
- **Verifications:**
  - ✓ Import completes successfully
  - ✓ Success count > 13,000 comments
  - ✓ Database count matches success count

### Step 6: Test Streaming Exports
Tests the synchronous streaming export endpoint with different parameters:

#### 6.1 Users (NDJSON)
- `GET /v1/exports?resource=users&format=ndjson`
- **Verifications:**
  - ✓ Export count matches import count
  - ✓ Measures rows/sec performance

#### 6.2 Users (CSV)
- `GET /v1/exports?resource=users&format=csv`
- **Verifications:**
  - ✓ Export count matches import count
  - ✓ CSV header is correct

#### 6.3 Articles (NDJSON)
- `GET /v1/exports?resource=articles&format=ndjson`
- **Verifications:**
  - ✓ Export count matches import count

#### 6.4 Comments (CSV)
- `GET /v1/exports?resource=comments&format=csv`
- **Verifications:**
  - ✓ Export count matches import count
  - ✓ Verifies cm_ prefix support

### Step 7: Test Async Exports with Filters
Tests the asynchronous export system with filtering:

#### 7.1 Published Articles (NDJSON)
- `POST /v1/exports` with `filter: {status: "published"}`
- Checks job status with `GET /v1/exports/{job_id}`
- **Verifications:**
  - ✓ Export completes successfully
  - ✓ Download URL is provided
  - ✓ Downloaded file matches record count
  - ✓ All records have status="published"

#### 7.2 Draft Articles (CSV)
- `POST /v1/exports` with `filter: {status: "draft"}`
- **Verifications:**
  - ✓ Export completes successfully
  - ✓ Record count is returned

#### 7.3 Idempotency Test
- Repeats request with same idempotency key
- **Verifications:**
  - ✓ Returns same job ID (no duplicate work)

### Step 8: Verify Metrics Collection
- Queries `api_metrics` table
- **Verifications:**
  - ✓ At least 10 metrics recorded
  - ✓ Metrics contain endpoint, duration, rows processed

## Test Output

The script provides colored output:
- 🔵 **BLUE**: Informational messages
- ✅ **GREEN**: Tests passed
- ❌ **RED**: Tests failed
- 🟡 **YELLOW**: Section headers

### Final Summary
```
Tests Passed: X
Tests Failed: Y
Total Time: Zs

✓ ALL TESTS PASSED
```

## Exit Codes
- `0`: All tests passed
- `1`: One or more tests failed

## Performance Expectations

The test suite verifies:
- **Imports**: Handle thousands of records per job
- **Streaming Exports**: > 5,000 rows/sec for NDJSON
- **Async Exports**: Complete within 30 seconds
- **Metrics**: All API calls tracked

## Customization

You can modify the test suite to:
- Add more filter combinations
- Test different export formats
- Verify specific validation rules
- Test error scenarios

## Troubleshooting

If tests fail:
1. Check server logs: `cat /tmp/server.log`
2. Verify test data exists in `import_testdata/`
3. Ensure no other process is using port 8080
4. Check database permissions

## Files Created During Tests

- `/tmp/users_export.ndjson` - Streaming user export
- `/tmp/users_export.csv` - CSV user export
- `/tmp/articles_export.ndjson` - Streaming article export
- `/tmp/comments_export.csv` - CSV comment export
- `/tmp/published_articles.ndjson` - Filtered async export
- `/tmp/server.log` - Server output logs

## Re-running Tests

The test suite is idempotent - you can run it multiple times:
```bash
./test-suite.sh
```

Each run starts with a clean database state.
