package imports

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bulk-import-export/articles"
	"bulk-import-export/common"
	"bulk-import-export/parsers"
	"bulk-import-export/users"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// BatchSize is the number of records processed in a single database transaction
	BatchSize = 2000

	// ProgressUpdateFrequency controls how often we save job progress to database
	// (every N batches). Set to 1 to update after every batch write
	ProgressUpdateFrequency = 1
)

// CreateImportRequest represents the request body for async imports
type CreateImportRequest struct {
	ResourceType string `json:"resource_type" binding:"required,oneof=users articles comments"`
	Format       string `json:"format" binding:"required,oneof=csv ndjson"`
	FileURL      string `json:"file_url"` // Optional: remote file URL
}

// CreateImportResponse represents the response for import job creation
type CreateImportResponse struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// GetImportResponse represents the response for import job status
type GetImportResponse struct {
	JobID          string                          `json:"job_id"`
	ResourceType   string                          `json:"resource_type"`
	Status         string                          `json:"status"`
	TotalRecords   int                             `json:"total_records"`
	ProcessedCount int                             `json:"processed_count"`
	SuccessCount   int                             `json:"success_count"`
	FailCount      int                             `json:"fail_count"`
	Errors         []common.RecordValidationResult `json:"errors,omitempty"`
	CreatedAt      string                          `json:"created_at"`
	UpdatedAt      string                          `json:"updated_at"`
	CompletedAt    *string                         `json:"completed_at,omitempty"`
}

// CreateImport godoc
// @Summary Create a new import job
// @Description Creates an import job to bulk import users, articles, or comments from CSV or NDJSON files
// @Tags imports
// @Accept multipart/form-data
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "Unique key to prevent duplicate imports"
// @Param file formData file false "File to import (CSV or NDJSON)"
// @Param resource_type formData string false "Type of resource (users, articles, or comments)"
// @Param file_url body string false "URL of file to import (alternative to file upload)"
// @Success 202 {object} CreateImportResponse "Import job created"
// @Success 200 {object} CreateImportResponse "Existing job returned (idempotency)"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /imports [post]
func CreateImport(c *gin.Context) {
	db := common.GetDB()

	// Get required idempotency key from header
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key header is required"})
		return
	}

	// Check for existing job with same idempotency key
	var existingJob common.ImportJob
	if err := db.Where("idempotency_key = ?", idempotencyKey).First(&existingJob).Error; err == nil {
		// Job already exists, return existing
		c.JSON(http.StatusOK, CreateImportResponse{
			JobID:     existingJob.ID,
			Status:    existingJob.Status,
			CreatedAt: existingJob.CreatedAt.Format(time.RFC3339),
		})
		return
	}

	var filePath string
	var resourceType string
	var format string

	// Check if it's multipart upload or JSON body
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle file upload
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
			return
		}
		defer file.Close()

		resourceType = c.PostForm("resource_type")
		if resourceType == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "resource_type is required"})
			return
		}

		// Determine format from filename
		ext := filepath.Ext(header.Filename)
		if ext == ".csv" {
			format = "csv"
		} else if ext == ".ndjson" || ext == ".json" {
			format = "ndjson"
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File must be .csv or .ndjson"})
			return
		}

		// Save file to uploads directory
		os.MkdirAll(common.UploadsDir, 0755)

		fileName := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102_150405"), uuid.New().String()[:8], ext)
		filePath = filepath.Join(common.UploadsDir, fileName)

		out, err := os.Create(filePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}
		defer out.Close()

		if _, err := io.Copy(out, file); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

	} else {
		// Handle JSON body with URL
		var req CreateImportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		resourceType = req.ResourceType
		format = req.Format

		if req.FileURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file_url is required"})
			return
		}

		// Download file from URL
		os.MkdirAll(common.UploadsDir, 0755)

		ext := ".ndjson"
		if format == "csv" {
			ext = ".csv"
		}

		fileName := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102_150405"), uuid.New().String()[:8], ext)
		filePath = filepath.Join(common.UploadsDir, fileName)

		if err := downloadFile(req.FileURL, filePath); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Failed to download file: %v", err)})
			return
		}
	}

	// Validate resource type
	if resourceType != "users" && resourceType != "articles" && resourceType != "comments" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource_type must be users, articles, or comments"})
		return
	}

	// Create import job
	job := common.ImportJob{
		ID:             uuid.New().String(),
		IdempotencyKey: idempotencyKey,
		ResourceType:   resourceType,
		Status:         common.JobStatusPending,
		FilePath:       filePath,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := db.Create(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create import job"})
		return
	}

	// Queue job for background processing
	go ProcessImportJob(job.ID)

	c.JSON(http.StatusAccepted, CreateImportResponse{
		JobID:     job.ID,
		Status:    job.Status,
		CreatedAt: job.CreatedAt.Format(time.RFC3339),
	})
}

// GetImport godoc
// @Summary Get import job status
// @Description Retrieves the status and progress of an import job
// @Tags imports
// @Produce json
// @Param job_id path string true "Import Job ID"
// @Success 200 {object} GetImportResponse "Import job details"
// @Failure 404 {object} map[string]string "Job not found"
// @Router /imports/{job_id} [get]
func GetImport(c *gin.Context) {
	db := common.GetDB()
	jobID := c.Param("job_id")

	var job common.ImportJob
	if err := db.Where("id = ?", jobID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Import job not found"})
		return
	}

	// Set rows processed for metrics
	c.Set("rows_processed", job.ProcessedCount)

	response := GetImportResponse{
		JobID:          job.ID,
		ResourceType:   job.ResourceType,
		Status:         job.Status,
		TotalRecords:   job.TotalRecords,
		ProcessedCount: job.ProcessedCount,
		SuccessCount:   job.SuccessCount,
		FailCount:      job.FailCount,
		CreatedAt:      job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      job.UpdatedAt.Format(time.RFC3339),
	}

	if job.CompletedAt != nil {
		completedStr := job.CompletedAt.Format(time.RFC3339)
		response.CompletedAt = &completedStr
	}

	// Parse errors JSON
	if job.Errors != "" {
		var errors []common.RecordValidationResult
		if err := json.Unmarshal([]byte(job.Errors), &errors); err == nil {
			response.Errors = errors
		}
	}

	c.JSON(http.StatusOK, response)
}

// downloadFile downloads a file from URL
func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// ProcessImportJob processes an import job in the background
func ProcessImportJob(jobID string) {
	db := common.GetDB()

	// Get job
	var job common.ImportJob
	if err := db.Where("id = ?", jobID).First(&job).Error; err != nil {
		return
	}

	// Update status to processing
	job.Status = common.JobStatusProcessing
	job.UpdatedAt = time.Now()
	db.Save(&job)

	// Open file
	file, err := os.Open(job.FilePath)
	if err != nil {
		job.Status = common.JobStatusFailed
		job.Errors = fmt.Sprintf(`[{"error": "Failed to open file: %v"}]`, err)
		now := time.Now()
		job.CompletedAt = &now
		job.UpdatedAt = now
		db.Save(&job)
		return
	}
	defer file.Close()

	// Process based on resource type
	var processErr error
	switch job.ResourceType {
	case "users":
		processErr = processUsersImport(&job, file)
	case "articles":
		processErr = processArticlesImport(&job, file)
	case "comments":
		processErr = processCommentsImport(&job, file)
	default:
		processErr = fmt.Errorf("unknown resource type: %s", job.ResourceType)
	}

	// Clean up orphaned records if processing succeeded
	if processErr == nil && (job.ResourceType == "articles" || job.ResourceType == "comments") {
		orphanedCount, orphanedErrors := cleanupOrphanedRecords(&job)
		if orphanedCount > 0 {
			// Update counts to reflect cleaned records
			job.SuccessCount -= orphanedCount
			job.FailCount += orphanedCount

			// Append orphaned record errors
			if len(orphanedErrors) > 0 {
				var existingErrors []common.RecordValidationResult
				if job.Errors != "" {
					json.Unmarshal([]byte(job.Errors), &existingErrors)
				}
				existingErrors = append(existingErrors, orphanedErrors...)
				errorsJSON, _ := json.Marshal(existingErrors)
				job.Errors = string(errorsJSON)
			}
		}
	}

	// Update job status
	now := time.Now()
	job.CompletedAt = &now
	job.UpdatedAt = now

	if processErr != nil {
		job.Status = common.JobStatusFailed
		if job.Errors == "" {
			job.Errors = fmt.Sprintf(`[{"error": "%v"}]`, processErr)
		}
	} else {
		job.Status = common.JobStatusCompleted
	}

	db.Save(&job)
}

// processUsersImport processes user import
func processUsersImport(job *common.ImportJob, file *os.File) error {
	db := common.GetDB()

	// Parse CSV
	records, errors := parsers.ParseCSV(file)
	go func() {
		for range errors {
		}
	}()

	// Collect records in batches
	var batch []map[string]string
	var allErrors []common.RecordValidationResult
	rowNum := 1

	validator := users.NewUserValidator()

	// Track batches processed to reduce DB updates
	batchesProcessed := 0

	for record := range records {
		batch = append(batch, record)
		job.TotalRecords++

		if len(batch) >= BatchSize {
			processedCount, failedResults := processBatchUsers(db, batch, validator, rowNum)
			job.ProcessedCount += processedCount
			job.SuccessCount += processedCount - len(failedResults)
			job.FailCount += len(failedResults)
			allErrors = append(allErrors, failedResults...)

			rowNum += len(batch)
			batch = nil

			// Update progress less frequently (every N batches)
			batchesProcessed++
			if batchesProcessed%ProgressUpdateFrequency == 0 {
				job.UpdatedAt = time.Now()
				db.Save(job)
			}
		}
	}

	// Process remaining batch
	if len(batch) > 0 {
		processedCount, failedResults := processBatchUsers(db, batch, validator, rowNum)
		job.ProcessedCount += processedCount
		job.SuccessCount += processedCount - len(failedResults)
		job.FailCount += len(failedResults)
		allErrors = append(allErrors, failedResults...)
	}

	// Store errors as JSON
	if len(allErrors) > 0 {
		errorsJSON, _ := json.Marshal(allErrors)
		job.Errors = string(errorsJSON)
	}

	return nil
}

// processBatchUsers validates and imports a batch of users
func processBatchUsers(db *gorm.DB, batch []map[string]string, validator *users.UserValidator, startRow int) (int, []common.RecordValidationResult) {
	var validUsers []users.UserModel
	var failedResults []common.RecordValidationResult

	// Validate each record
	for i, record := range batch {
		result := validator.ValidateUserRecord(record, startRow+i)

		if result.Valid {
			// Check if record has email (natural key)
			email := strings.TrimSpace(record["email"])
			if email == "" {
				result.Valid = false
				result.AddError("email", "Email is required (natural key for upsert)")
				failedResults = append(failedResults, *result)
				log.Printf("Rejected user without email at row %d", startRow+i)
				continue
			}

			user := users.NormalizeUserRecord(record)
			validUsers = append(validUsers, user)
		} else {
			failedResults = append(failedResults, *result)
		}
	}

	// Upsert by email (natural key)
	if len(validUsers) > 0 {
		db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "email"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "role", "active", "updated_at"}),
		}).Create(&validUsers)
	}

	return len(batch), failedResults
}

// processArticlesImport processes article import
func processArticlesImport(job *common.ImportJob, file *os.File) error {
	db := common.GetDB()

	// Parse NDJSON
	records, errors := parsers.ParseNDJSON(file)
	go func() {
		for range errors {
		}
	}()

	// Collect records in batches
	var batch []map[string]interface{}
	var allErrors []common.RecordValidationResult
	rowNum := 1

	validator := articles.NewArticleValidator()

	// Track batches processed to reduce DB updates
	batchesProcessed := 0

	for record := range records {
		batch = append(batch, record)
		job.TotalRecords++

		if len(batch) >= BatchSize {
			processedCount, failedResults := processBatchArticles(db, batch, validator, rowNum)
			job.ProcessedCount += processedCount
			job.SuccessCount += processedCount - len(failedResults)
			job.FailCount += len(failedResults)
			allErrors = append(allErrors, failedResults...)

			rowNum += len(batch)
			batch = nil

			// Update progress less frequently
			batchesProcessed++
			if batchesProcessed%ProgressUpdateFrequency == 0 {
				job.UpdatedAt = time.Now()
				db.Save(job)
			}
		}
	}

	// Process remaining batch
	if len(batch) > 0 {
		processedCount, failedResults := processBatchArticles(db, batch, validator, rowNum)
		job.ProcessedCount += processedCount
		job.SuccessCount += processedCount - len(failedResults)
		job.FailCount += len(failedResults)
		allErrors = append(allErrors, failedResults...)
	}

	// Store errors as JSON
	if len(allErrors) > 0 {
		errorsJSON, _ := json.Marshal(allErrors)
		job.Errors = string(errorsJSON)
	}

	return nil
}

// processBatchArticles validates and imports a batch of articles
func processBatchArticles(db *gorm.DB, batch []map[string]interface{}, validator *articles.ArticleValidator, startRow int) (int, []common.RecordValidationResult) {
	var validArticles []articles.ArticleModel
	var failedResults []common.RecordValidationResult

	// Validate each record
	for i, record := range batch {
		result := validator.ValidateArticleRecord(record, startRow+i)

		if result.Valid {
			// Check if record has slug (natural key)
			slug := ""
			if slugVal, ok := record["slug"]; ok && slugVal != nil {
				slug = strings.TrimSpace(fmt.Sprint(slugVal))
			}

			if slug == "" {
				result.Valid = false
				result.AddError("slug", "Slug is required (natural key for upsert)")
				failedResults = append(failedResults, *result)
				log.Printf("Rejected article without slug at row %d", startRow+i)
				continue
			}

			article := articles.NormalizeArticleRecord(record)
			validArticles = append(validArticles, article)
		} else {
			failedResults = append(failedResults, *result)
		}
	}

	// Upsert by slug (natural key)
	if len(validArticles) > 0 {
		db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "body", "author_id", "tags", "status", "published_at"}),
		}).Create(&validArticles)
	}

	return len(batch), failedResults
}

// processCommentsImport processes comment import
func processCommentsImport(job *common.ImportJob, file *os.File) error {
	db := common.GetDB()

	// Parse NDJSON
	records, errors := parsers.ParseNDJSON(file)
	go func() {
		for range errors {
		}
	}()

	// Collect records in batches
	var batch []map[string]interface{}
	var allErrors []common.RecordValidationResult
	rowNum := 1

	validator := articles.NewCommentValidator()

	// Track batches processed to reduce DB updates
	batchesProcessed := 0

	for record := range records {
		batch = append(batch, record)
		job.TotalRecords++

		if len(batch) >= BatchSize {
			processedCount, failedResults := processBatchComments(db, batch, validator, rowNum)
			job.ProcessedCount += processedCount
			job.SuccessCount += processedCount - len(failedResults)
			job.FailCount += len(failedResults)
			allErrors = append(allErrors, failedResults...)

			rowNum += len(batch)
			batch = nil

			// Update progress less frequently
			batchesProcessed++
			if batchesProcessed%ProgressUpdateFrequency == 0 {
				job.UpdatedAt = time.Now()
				db.Save(job)
			}
		}
	}

	// Process remaining batch
	if len(batch) > 0 {
		processedCount, failedResults := processBatchComments(db, batch, validator, rowNum)
		job.ProcessedCount += processedCount
		job.SuccessCount += processedCount - len(failedResults)
		job.FailCount += len(failedResults)
		allErrors = append(allErrors, failedResults...)
	}

	// Store errors as JSON
	if len(allErrors) > 0 {
		errorsJSON, _ := json.Marshal(allErrors)
		job.Errors = string(errorsJSON)
	}

	return nil
}

// processBatchComments validates and imports a batch of comments
func processBatchComments(db *gorm.DB, batch []map[string]interface{}, validator *articles.CommentValidator, startRow int) (int, []common.RecordValidationResult) {
	var validComments []articles.CommentModel
	var failedResults []common.RecordValidationResult

	// Validate each record
	for i, record := range batch {
		result := validator.ValidateCommentRecord(record, startRow+i)

		if result.Valid {
			// Check if record has an ID
			origID := ""
			if idVal, ok := record["id"]; ok && idVal != nil {
				origID = strings.TrimSpace(fmt.Sprint(idVal))
			}

			// Reject comments without ID (no natural key exists for comments)
			if origID == "" {
				result.Valid = false
				result.AddError("id", "ID is required for comments (no natural key available)")
				failedResults = append(failedResults, *result)
				log.Printf("Rejected comment without ID at row %d", startRow+i)
				continue
			}

			comment := articles.NormalizeCommentRecord(record)
			validComments = append(validComments, comment)
		} else {
			failedResults = append(failedResults, *result)
		}
	}

	// Bulk upsert valid comments by ID
	if len(validComments) > 0 {
		db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"article_id", "user_id", "body", "created_at"}),
		}).Create(&validComments)
	}

	return len(batch), failedResults
}

// cleanupOrphanedRecords finds and removes records with invalid foreign keys
func cleanupOrphanedRecords(job *common.ImportJob) (int, []common.RecordValidationResult) {
	db := common.GetDB()
	var failedResults []common.RecordValidationResult
	totalOrphaned := 0

	if job.ResourceType == "articles" {
		// Find articles with invalid author_id
		var orphanedArticles []struct {
			ID       string
			AuthorID string
		}

		db.Raw(`
			SELECT a.id, a.author_id 
			FROM articles a 
			LEFT JOIN users u ON a.author_id = u.id 
			WHERE u.id IS NULL
		`).Scan(&orphanedArticles)

		if len(orphanedArticles) > 0 {
			log.Printf("Found %d orphaned articles (invalid author_id)", len(orphanedArticles))

			// Delete orphaned articles and log each one
			for _, article := range orphanedArticles {
				db.Exec("DELETE FROM articles WHERE id = ?", article.ID)

				result := common.RecordValidationResult{
					RecordID: article.ID,
					Valid:    false,
				}
				result.AddError("author_id", fmt.Sprintf("Foreign key violation: author_id '%s' does not exist", article.AuthorID))
				failedResults = append(failedResults, result)

				log.Printf("Deleted orphaned article %s (author_id: %s)", article.ID, article.AuthorID)
			}

			totalOrphaned += len(orphanedArticles)
		}
	}

	if job.ResourceType == "comments" {
		// Find comments with invalid article_id or user_id
		var orphanedComments []struct {
			ID        string
			ArticleID string
			UserID    string
		}

		db.Raw(`
			SELECT c.id, c.article_id, c.user_id
			FROM comments c
			LEFT JOIN articles a ON c.article_id = a.id
			LEFT JOIN users u ON c.user_id = u.id
			WHERE a.id IS NULL OR u.id IS NULL
		`).Scan(&orphanedComments)

		if len(orphanedComments) > 0 {
			log.Printf("Found %d orphaned comments (invalid article_id or user_id)", len(orphanedComments))

			// Delete orphaned comments and log each one
			for _, comment := range orphanedComments {
				db.Exec("DELETE FROM comments WHERE id = ?", comment.ID)

				result := common.RecordValidationResult{
					RecordID: comment.ID,
					Valid:    false,
				}

				// Check which FK is invalid
				var articleExists, userExists bool
				db.Raw("SELECT EXISTS(SELECT 1 FROM articles WHERE id = ?)", comment.ArticleID).Scan(&articleExists)
				db.Raw("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", comment.UserID).Scan(&userExists)

				if !articleExists {
					result.AddError("article_id", fmt.Sprintf("Foreign key violation: article_id '%s' does not exist", comment.ArticleID))
				}
				if !userExists {
					result.AddError("user_id", fmt.Sprintf("Foreign key violation: user_id '%s' does not exist", comment.UserID))
				}

				failedResults = append(failedResults, result)

				log.Printf("Deleted orphaned comment %s (article_id: %s, user_id: %s)", comment.ID, comment.ArticleID, comment.UserID)
			}

			totalOrphaned += len(orphanedComments)
		}
	}

	return totalOrphaned, failedResults
}
