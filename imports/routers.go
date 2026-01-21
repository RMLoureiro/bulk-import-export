package imports

import (
	"encoding/json"
	"fmt"
	"io"
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
	JobID          string                 `json:"job_id"`
	ResourceType   string                 `json:"resource_type"`
	Status         string                 `json:"status"`
	TotalRecords   int                    `json:"total_records"`
	ProcessedCount int                    `json:"processed_count"`
	SuccessCount   int                    `json:"success_count"`
	FailCount      int                    `json:"fail_count"`
	Errors         []common.RecordValidationResult `json:"errors,omitempty"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
	CompletedAt    *string                `json:"completed_at,omitempty"`
}

// CreateImport handles POST /v1/imports
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
		uploadsDir := "./data/uploads"
		os.MkdirAll(uploadsDir, 0755)
		
		fileName := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102_150405"), uuid.New().String()[:8], ext)
		filePath = filepath.Join(uploadsDir, fileName)
		
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
		uploadsDir := "./data/uploads"
		os.MkdirAll(uploadsDir, 0755)
		
		ext := ".ndjson"
		if format == "csv" {
			ext = ".csv"
		}
		
		fileName := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102_150405"), uuid.New().String()[:8], ext)
		filePath = filepath.Join(uploadsDir, fileName)
		
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
		Status:         "pending",
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

// GetImport handles GET /v1/imports/:job_id
func GetImport(c *gin.Context) {
	db := common.GetDB()
	jobID := c.Param("job_id")
	
	var job common.ImportJob
	if err := db.Where("id = ?", jobID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Import job not found"})
		return
	}
	
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
	job.Status = "processing"
	job.UpdatedAt = time.Now()
	db.Save(&job)
	
	// Open file
	file, err := os.Open(job.FilePath)
	if err != nil {
		job.Status = "failed"
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
	
	// Update job status
	now := time.Now()
	job.CompletedAt = &now
	job.UpdatedAt = now
	
	if processErr != nil {
		job.Status = "failed"
		if job.Errors == "" {
			job.Errors = fmt.Sprintf(`[{"error": "%v"}]`, processErr)
		}
	} else {
		job.Status = "completed"
	}
	
	db.Save(&job)
}

// processUsersImport processes user import
func processUsersImport(job *common.ImportJob, file *os.File) error {
	db := common.GetDB()
	
	// Parse CSV
	records, errors := parsers.ParseCSV(file)
	go func() {
		for range errors {}
	}()
	
	// Collect records in batches
	batchSize := 1000
	var batch []map[string]string
	var allErrors []common.RecordValidationResult
	rowNum := 1
	
	validator := users.NewUserValidator()
	
	for record := range records {
		batch = append(batch, record)
		job.TotalRecords++
		
		if len(batch) >= batchSize {
			processedCount, failedResults := processBatchUsers(db, batch, validator, rowNum)
			job.ProcessedCount += processedCount
			job.SuccessCount += processedCount - len(failedResults)
			job.FailCount += len(failedResults)
			allErrors = append(allErrors, failedResults...)
			
			rowNum += len(batch)
			batch = nil
			
			// Update progress
			job.UpdatedAt = time.Now()
			db.Save(job)
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
			user := users.NormalizeUserRecord(record)
			validUsers = append(validUsers, user)
		} else {
			failedResults = append(failedResults, *result)
		}
	}
	
	// Upsert valid users
	if len(validUsers) > 0 {
		for _, user := range validUsers {
			// Upsert by email (natural key)
			db.Where("email = ?", user.Email).Assign(user).FirstOrCreate(&user)
		}
	}
	
	return len(batch), failedResults
}

// processArticlesImport processes article import
func processArticlesImport(job *common.ImportJob, file *os.File) error {
	db := common.GetDB()
	
	// Parse NDJSON
	records, errors := parsers.ParseNDJSON(file)
	go func() {
		for range errors {}
	}()
	
	// Collect records in batches
	batchSize := 1000
	var batch []map[string]interface{}
	var allErrors []common.RecordValidationResult
	rowNum := 1
	
	validator := articles.NewArticleValidator()
	
	for record := range records {
		batch = append(batch, record)
		job.TotalRecords++
		
		if len(batch) >= batchSize {
			processedCount, failedResults := processBatchArticles(db, batch, validator, rowNum)
			job.ProcessedCount += processedCount
			job.SuccessCount += processedCount - len(failedResults)
			job.FailCount += len(failedResults)
			allErrors = append(allErrors, failedResults...)
			
			rowNum += len(batch)
			batch = nil
			
			// Update progress
			job.UpdatedAt = time.Now()
			db.Save(job)
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
	var articleTags []map[string][]string // map article index to tag names
	var failedResults []common.RecordValidationResult
	
	// Validate each record
	for i, record := range batch {
		result := validator.ValidateArticleRecord(record, startRow+i)
		
		if result.Valid {
			article, tags := articles.NormalizeArticleRecord(record)
			validArticles = append(validArticles, article)
			if len(tags) > 0 {
				articleTags = append(articleTags, map[string][]string{
					fmt.Sprintf("%d", len(validArticles)-1): tags,
				})
			}
		} else {
			failedResults = append(failedResults, *result)
		}
	}
	
	// Upsert valid articles
	if len(validArticles) > 0 {
		for idx, article := range validArticles {
			// Upsert by slug (natural key)
			db.Where("slug = ?", article.Slug).Assign(article).FirstOrCreate(&article)
			
			// Handle tags
			var tagNames []string
			for _, tagMap := range articleTags {
				if tags, ok := tagMap[fmt.Sprintf("%d", idx)]; ok {
					tagNames = tags
					break
				}
			}
			
			if len(tagNames) > 0 {
				tagModels, _ := articles.UpsertTags(tagNames)
				if len(tagModels) > 0 {
					db.Model(&article).Association("Tags").Replace(tagModels)
				}
			}
		}
	}
	
	return len(batch), failedResults
}

// processCommentsImport processes comment import
func processCommentsImport(job *common.ImportJob, file *os.File) error {
	db := common.GetDB()
	
	// Parse NDJSON
	records, errors := parsers.ParseNDJSON(file)
	go func() {
		for range errors {}
	}()
	
	// Collect records in batches
	batchSize := 1000
	var batch []map[string]interface{}
	var allErrors []common.RecordValidationResult
	rowNum := 1
	
	validator := articles.NewCommentValidator()
	
	for record := range records {
		batch = append(batch, record)
		job.TotalRecords++
		
		if len(batch) >= batchSize {
			processedCount, failedResults := processBatchComments(db, batch, validator, rowNum)
			job.ProcessedCount += processedCount
			job.SuccessCount += processedCount - len(failedResults)
			job.FailCount += len(failedResults)
			allErrors = append(allErrors, failedResults...)
			
			rowNum += len(batch)
			batch = nil
			
			// Update progress
			job.UpdatedAt = time.Now()
			db.Save(job)
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
			comment := articles.NormalizeCommentRecord(record)
			validComments = append(validComments, comment)
		} else {
			failedResults = append(failedResults, *result)
		}
	}
	
	// Insert valid comments (comments don't have natural key, use ID for upsert)
	if len(validComments) > 0 {
		for _, comment := range validComments {
			db.Where("id = ?", comment.ID).Assign(comment).FirstOrCreate(&comment)
		}
	}
	
	return len(batch), failedResults
}
