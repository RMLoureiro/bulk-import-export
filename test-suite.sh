#!/bin/bash

# Comprehensive Test Suite for Bulk Import/Export
# This script tests the complete import/export workflow

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

log_section() {
    echo -e "\n${YELLOW}========================================${NC}"
    echo -e "${YELLOW}$1${NC}"
    echo -e "${YELLOW}========================================${NC}\n"
}

assert_equals() {
    if [ "$1" == "$2" ]; then
        log_success "$3"
    else
        log_error "$3 (Expected: $2, Got: $1)"
    fi
}

assert_greater_than() {
    if [ "$1" -gt "$2" ]; then
        log_success "$3"
    else
        log_error "$3 (Expected > $2, Got: $1)"
    fi
}

wait_for_job() {
    local job_id=$1
    local endpoint=$2
    local last_count=0
    local check_number=0
    
    while true; do
        check_number=$((check_number + 1))
        
        # Get job status and processed count - print immediately (not via log_info)
        echo -e "${BLUE}[INFO]${NC} Check #${check_number}: Calling GET /v1/${endpoint}/${job_id}" >&2
        response=$(curl -s http://localhost:8080/v1/${endpoint}/${job_id})
        status=$(echo "$response" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])" 2>/dev/null)
        
        # Get current progress count (processed_count for imports, total_records for exports)
        if [ "$endpoint" == "imports" ]; then
            current_count=$(echo "$response" | python3 -c "import sys,json; print(json.load(sys.stdin).get('processed_count', 0))" 2>/dev/null)
        else
            current_count=$(echo "$response" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total_records', 0))" 2>/dev/null)
        fi
        
        echo -e "${BLUE}[INFO]${NC} Check #${check_number}: Status=${status}, Records processed=${current_count}" >&2
        
        # Check if job is finished
        if [ "$status" == "completed" ] || [ "$status" == "failed" ]; then
            echo "$status"
            return
        fi
        
        # Check if progress is being made
        if [ $check_number -eq 1 ]; then
            echo -e "${BLUE}[INFO]${NC} First check completed - waiting 60 seconds for next check..." >&2
            last_count=$current_count
        elif [ "$current_count" -gt "$last_count" ]; then
            progress=$((current_count - last_count))
            echo -e "${BLUE}[INFO]${NC} Progress detected: +${progress} records since last check (${last_count} → ${current_count})" >&2
            echo -e "${BLUE}[INFO]${NC} Waiting 60 seconds for next check..." >&2
            last_count=$current_count
        elif [ "$current_count" -eq "$last_count" ]; then
            echo -e "${RED}[ERROR]${NC} No progress in last 60 seconds (stuck at $current_count records)" >&2
            echo "stalled"
            return
        fi
        
        # Wait 60 seconds before next check
        sleep 60
    done
}

# Start test suite
log_section "BULK IMPORT/EXPORT TEST SUITE"
START_TIME=$(date +%s)

# Step 1: Stop server and clean database
log_section "STEP 1: Clean Database"
log_info "Stopping any running server..."
pkill -f bulk-import-export || true
sleep 2

log_info "Removing database..."
rm -f data/gorm.db
log_success "Database cleaned"

log_info "Clearing upload and export directories..."
rm -rf data/uploads/* data/exports/*
log_success "Directories cleaned"

# Step 2: Start server
log_section "STEP 2: Start Server"
log_info "Building application..."
go build
log_success "Build completed"

log_info "Starting server in background..."
./bulk-import-export > /tmp/server.log 2>&1 &
SERVER_PID=$!
sleep 3

# Check if server is running
if ! curl -s http://localhost:8080/health > /dev/null; then
    log_error "Server failed to start"
    cat /tmp/server.log
    exit 1
fi
log_success "Server started (PID: $SERVER_PID)"

# Step 3: Import Users
log_section "STEP 3: Import Users"
log_info "Starting user import..."

USER_RESPONSE=$(curl -s -X POST http://localhost:8080/v1/imports \
  -H "Idempotency-Key: test-users-001" \
  -F "file=@import_testdata/users_huge.csv" \
  -F "resource_type=users")

USER_JOB_ID=$(echo $USER_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")
log_info "User import job created: $USER_JOB_ID"

# Check import status while processing
log_info "Monitoring import status..."
sleep 2
USER_STATUS_RESPONSE=$(curl -s http://localhost:8080/v1/imports/${USER_JOB_ID})
USER_PROCESSED=$(echo $USER_STATUS_RESPONSE | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('processed_count', 0))")
log_info "Processing... $USER_PROCESSED records processed"

# Wait for completion
log_info "Waiting for user import to complete..."
USER_FINAL_STATUS=$(wait_for_job $USER_JOB_ID "imports")

if [ "$USER_FINAL_STATUS" == "completed" ]; then
    USER_FINAL_RESPONSE=$(curl -s http://localhost:8080/v1/imports/${USER_JOB_ID})
    USER_SUCCESS=$(echo $USER_FINAL_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['success_count'])")
    USER_FAIL=$(echo $USER_FINAL_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['fail_count'])")
    
    log_success "User import completed: $USER_SUCCESS succeeded, $USER_FAIL failed"
    assert_greater_than $USER_SUCCESS 9000 "Imported > 9000 users"
else
    log_error "User import failed with status: $USER_FINAL_STATUS"
fi

# Verify user data in database
USER_COUNT=$(sqlite3 data/gorm.db "SELECT COUNT(*) FROM users")
assert_equals $USER_COUNT $USER_SUCCESS "User count in database matches success count"

# Step 4: Import Articles
log_section "STEP 4: Import Articles"
log_info "Starting article import..."

ARTICLE_RESPONSE=$(curl -s -X POST http://localhost:8080/v1/imports \
  -H "Idempotency-Key: test-articles-001" \
  -F "file=@import_testdata/articles_huge.ndjson" \
  -F "resource_type=articles")

ARTICLE_JOB_ID=$(echo $ARTICLE_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")
log_info "Article import job created: $ARTICLE_JOB_ID"

# Wait for completion
log_info "Waiting for article import to complete..."
ARTICLE_FINAL_STATUS=$(wait_for_job $ARTICLE_JOB_ID "imports")

if [ "$ARTICLE_FINAL_STATUS" == "completed" ]; then
    ARTICLE_FINAL_RESPONSE=$(curl -s http://localhost:8080/v1/imports/${ARTICLE_JOB_ID})
    ARTICLE_SUCCESS=$(echo $ARTICLE_FINAL_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['success_count'])")
    ARTICLE_FAIL=$(echo $ARTICLE_FINAL_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['fail_count'])")
    
    log_success "Article import completed: $ARTICLE_SUCCESS succeeded, $ARTICLE_FAIL failed"
    assert_greater_than $ARTICLE_SUCCESS 10000 "Imported > 10000 articles"
else
    log_error "Article import failed with status: $ARTICLE_FINAL_STATUS"
fi

# Verify article data
ARTICLE_COUNT=$(sqlite3 data/gorm.db "SELECT COUNT(*) FROM articles")
assert_equals $ARTICLE_COUNT $ARTICLE_SUCCESS "Article count in database matches success count"

# Step 5: Import Comments
log_section "STEP 5: Import Comments"
log_info "Starting comment import..."

COMMENT_RESPONSE=$(curl -s -X POST http://localhost:8080/v1/imports \
  -H "Idempotency-Key: test-comments-001" \
  -F "file=@import_testdata/comments_huge.ndjson" \
  -F "resource_type=comments")

COMMENT_JOB_ID=$(echo $COMMENT_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")
log_info "Comment import job created: $COMMENT_JOB_ID"

# Wait for completion
log_info "Waiting for comment import to complete..."
COMMENT_FINAL_STATUS=$(wait_for_job $COMMENT_JOB_ID "imports")

if [ "$COMMENT_FINAL_STATUS" == "completed" ]; then
    COMMENT_FINAL_RESPONSE=$(curl -s http://localhost:8080/v1/imports/${COMMENT_JOB_ID})
    COMMENT_SUCCESS=$(echo $COMMENT_FINAL_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['success_count'])")
    COMMENT_FAIL=$(echo $COMMENT_FINAL_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['fail_count'])")
    
    log_success "Comment import completed: $COMMENT_SUCCESS succeeded, $COMMENT_FAIL failed"
    assert_greater_than $COMMENT_SUCCESS 13000 "Imported > 13000 comments"
else
    log_error "Comment import failed with status: $COMMENT_FINAL_STATUS"
fi

# Verify comment data
COMMENT_COUNT=$(sqlite3 data/gorm.db "SELECT COUNT(*) FROM comments")
assert_equals $COMMENT_COUNT $COMMENT_SUCCESS "Comment count in database matches success count"

# Step 6: Test Streaming Exports
log_section "STEP 6: Test Streaming Exports"

# Test 6.1: Export users as NDJSON
log_info "Testing streaming export: users (NDJSON)..."
STREAM_START=$(date +%s%3N)
curl -s "http://localhost:8080/v1/exports?resource=users&format=ndjson" > /tmp/users_export.ndjson
STREAM_END=$(date +%s%3N)
STREAM_DURATION=$((STREAM_END - STREAM_START))

EXPORT_USER_COUNT=$(wc -l < /tmp/users_export.ndjson)
assert_equals $EXPORT_USER_COUNT $USER_SUCCESS "Streaming export user count matches import"
ROWS_PER_SEC=$((EXPORT_USER_COUNT * 1000 / STREAM_DURATION))
log_info "Performance: $ROWS_PER_SEC rows/sec"

# Test 6.2: Export users as CSV
log_info "Testing streaming export: users (CSV)..."
curl -s "http://localhost:8080/v1/exports?resource=users&format=csv" > /tmp/users_export.csv
CSV_LINE_COUNT=$(wc -l < /tmp/users_export.csv)
# CSV has header row, so subtract 1
CSV_USER_COUNT=$((CSV_LINE_COUNT - 1))
assert_equals $CSV_USER_COUNT $USER_SUCCESS "CSV export user count matches import"

# Verify CSV header
CSV_HEADER=$(head -1 /tmp/users_export.csv)
if [[ "$CSV_HEADER" == "id,email,name,role,active,created_at,updated_at" ]]; then
    log_success "CSV header is correct"
else
    log_error "CSV header is incorrect: $CSV_HEADER"
fi

# Test 6.3: Export articles as NDJSON
log_info "Testing streaming export: articles (NDJSON)..."
curl -s "http://localhost:8080/v1/exports?resource=articles&format=ndjson" > /tmp/articles_export.ndjson
EXPORT_ARTICLE_COUNT=$(wc -l < /tmp/articles_export.ndjson)
assert_equals $EXPORT_ARTICLE_COUNT $ARTICLE_SUCCESS "Streaming export article count matches import"

# Test 6.4: Export comments as CSV
log_info "Testing streaming export: comments (CSV)..."
curl -s "http://localhost:8080/v1/exports?resource=comments&format=csv" > /tmp/comments_export.csv
CSV_COMMENT_COUNT=$(($(wc -l < /tmp/comments_export.csv) - 1))
assert_equals $CSV_COMMENT_COUNT $COMMENT_SUCCESS "CSV export comment count matches import"

# Verify cm_ prefix in comments
CM_PREFIX_COUNT=$(grep -c '"id":"cm_' /tmp/comments_export.ndjson 2>/dev/null || echo 0)
log_info "Comments with cm_ prefix: $CM_PREFIX_COUNT"

# Step 7: Test Async Exports with Filters
log_section "STEP 7: Test Async Exports with Filters"

# Test 7.1: Export published articles only
log_info "Testing async export: published articles (NDJSON)..."
PUBLISHED_RESPONSE=$(curl -s -X POST http://localhost:8080/v1/exports \
  -H "Content-Type: application/json" \
  -d '{"idempotency_key":"test-published-001","resource_type":"articles","format":"ndjson","filters":{"status":"published"}}')

PUBLISHED_JOB_ID=$(echo $PUBLISHED_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")
log_info "Published articles export job: $PUBLISHED_JOB_ID"

# Wait for export to complete
log_info "Waiting for export to complete..."
EXPORT_STATUS=$(wait_for_job $PUBLISHED_JOB_ID "exports")

if [ "$EXPORT_STATUS" == "completed" ]; then
    EXPORT_RESPONSE=$(curl -s http://localhost:8080/v1/exports/${PUBLISHED_JOB_ID})
    PUBLISHED_COUNT=$(echo $EXPORT_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['total_records'])")
    DOWNLOAD_URL=$(echo $EXPORT_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['download_url'])")
    
    log_success "Published articles exported: $PUBLISHED_COUNT records"
    log_info "Download URL: $DOWNLOAD_URL"
    
    # Download and verify file
    curl -s "http://localhost:8080${DOWNLOAD_URL}" > /tmp/published_articles.ndjson
    FILE_LINE_COUNT=$(wc -l < /tmp/published_articles.ndjson)
    assert_equals $FILE_LINE_COUNT $PUBLISHED_COUNT "Downloaded file matches record count"
    
    # Verify all articles have status=published
    NON_PUBLISHED=$(grep -v '"status":"published"' /tmp/published_articles.ndjson | wc -l)
    assert_equals $NON_PUBLISHED 0 "All exported articles have published status"
else
    log_error "Async export failed with status: $EXPORT_STATUS"
fi

# Test 7.2: Export draft articles as CSV
log_info "Testing async export: draft articles (CSV)..."
DRAFT_RESPONSE=$(curl -s -X POST http://localhost:8080/v1/exports \
  -H "Content-Type: application/json" \
  -d '{"idempotency_key":"test-draft-001","resource_type":"articles","format":"csv","filters":{"status":"draft"}}')

DRAFT_JOB_ID=$(echo $DRAFT_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")

DRAFT_STATUS=$(wait_for_job $DRAFT_JOB_ID "exports")
if [ "$DRAFT_STATUS" == "completed" ]; then
    DRAFT_RESPONSE=$(curl -s http://localhost:8080/v1/exports/${DRAFT_JOB_ID})
    DRAFT_COUNT=$(echo $DRAFT_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['total_records'])")
    log_success "Draft articles exported: $DRAFT_COUNT records"
else
    log_error "Draft export failed"
fi

# Test 7.3: Test idempotency - same key should return existing job
log_info "Testing idempotency..."
DUPLICATE_RESPONSE=$(curl -s -X POST http://localhost:8080/v1/exports \
  -H "Content-Type: application/json" \
  -d '{"idempotency_key":"test-published-001","resource_type":"articles","format":"ndjson","filters":{"status":"published"}}')

DUPLICATE_JOB_ID=$(echo $DUPLICATE_RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")
if [ "$DUPLICATE_JOB_ID" == "$PUBLISHED_JOB_ID" ]; then
    log_success "Idempotency working: same job ID returned"
else
    log_error "Idempotency failed: different job ID ($DUPLICATE_JOB_ID vs $PUBLISHED_JOB_ID)"
fi

# Step 8: Verify Metrics
log_section "STEP 8: Verify Metrics Collection"

# Wait a moment for any pending metrics to be written
sleep 2

# Retry logic for database locked errors
METRIC_COUNT=""
for i in {1..5}; do
    METRIC_COUNT=$(sqlite3 data/gorm.db "SELECT COUNT(*) FROM api_metrics" 2>/dev/null)
    if [ $? -eq 0 ] && [ -n "$METRIC_COUNT" ]; then
        break
    fi
    log_info "Database locked, retrying in 1 second... (attempt $i/5)"
    sleep 1
done

if [ -z "$METRIC_COUNT" ]; then
    log_error "Failed to query metrics after 5 attempts"
    METRIC_COUNT=0
fi

log_info "Total API metrics recorded: $METRIC_COUNT"
assert_greater_than $METRIC_COUNT 10 "Metrics are being collected"

# Check for request IDs
REQUEST_ID_SAMPLE=$(sqlite3 data/gorm.db "SELECT endpoint, duration_ms FROM api_metrics ORDER BY timestamp DESC LIMIT 1" 2>/dev/null)
log_info "Latest metric: $REQUEST_ID_SAMPLE"

# Test Summary
log_section "TEST SUMMARY"
END_TIME=$(date +%s)
TOTAL_TIME=$((END_TIME - START_TIME))

echo -e "${GREEN}Tests Passed: $TESTS_PASSED${NC}"
echo -e "${RED}Tests Failed: $TESTS_FAILED${NC}"
echo -e "${BLUE}Total Time: ${TOTAL_TIME}s${NC}"

# Cleanup
log_info "Stopping server..."
kill $SERVER_PID 2>/dev/null || true

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "\n${GREEN}✓ ALL TESTS PASSED${NC}\n"
    exit 0
else
    echo -e "\n${RED}✗ SOME TESTS FAILED${NC}\n"
    exit 1
fi
