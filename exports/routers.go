package exports

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bulk-import-export/articles"
	"bulk-import-export/common"
	"bulk-import-export/users"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StreamExport godoc
// @Summary Stream export data (synchronous)
// @Description Streams users, articles, or comments directly in CSV or NDJSON format
// @Tags exports
// @Produce text/csv
// @Produce application/x-ndjson
// @Param resource query string true "Resource type (users, articles, or comments)"
// @Param format query string true "Export format (csv or ndjson)"
// @Success 200 {file} file "Streaming export data"
// @Failure 400 {object} map[string]string "Bad request"
// @Router /exports [get]
func StreamExport(c *gin.Context) {
	resource := c.Query("resource")
	format := c.Query("format")

	// Validate parameters
	if resource == "" {
		c.JSON(400, gin.H{"error": "resource parameter is required (users|articles|comments)"})
		return
	}
	if format == "" {
		c.JSON(400, gin.H{"error": "format parameter is required (csv|ndjson)"})
		return
	}

	validResources := map[string]bool{"users": true, "articles": true, "comments": true}
	if !validResources[resource] {
		c.JSON(400, gin.H{"error": "invalid resource, must be: users, articles, or comments"})
		return
	}

	validFormats := map[string]bool{"csv": true, "ndjson": true}
	if !validFormats[format] {
		c.JSON(400, gin.H{"error": "invalid format, must be: csv or ndjson"})
		return
	}

	// Set appropriate headers for streaming
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.%s", resource, timestamp, format)
	
	if format == "csv" {
		c.Header("Content-Type", "text/csv")
	} else {
		c.Header("Content-Type", "application/x-ndjson")
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Transfer-Encoding", "chunked")

	// Stream the data
	c.Stream(func(w io.Writer) bool {
		db := common.GetDB()
		
		switch resource {
		case "users":
			streamUsers(w, db, format, c)
		case "articles":
			streamArticles(w, db, format, c)
		case "comments":
			streamComments(w, db, format, c)
		}
		
		return false // Done streaming
	})
}

// streamUsers streams user data in the specified format
func streamUsers(w io.Writer, db *gorm.DB, format string, c *gin.Context) {
	const batchSize = 1000
	offset := 0

	if format == "csv" {
		csvWriter := csv.NewWriter(w)
		// Write header
		csvWriter.Write([]string{"id", "email", "name", "role", "active", "created_at", "updated_at"})
		csvWriter.Flush()

		for {
			var users []users.UserModel
			result := db.Limit(batchSize).Offset(offset).Find(&users)
			if result.Error != nil || len(users) == 0 {
				break
			}

			for _, user := range users {
				csvWriter.Write([]string{
					user.ID,
					user.Email,
					user.Name,
					user.Role,
					fmt.Sprintf("%t", user.Active),
					user.CreatedAt.Format(time.RFC3339),
					user.UpdatedAt.Format(time.RFC3339),
				})
			}
			csvWriter.Flush()
			
			if len(users) < batchSize {
				break
			}
			offset += batchSize
		}
	} else {
		// NDJSON format
		for {
			var users []users.UserModel
			result := db.Limit(batchSize).Offset(offset).Find(&users)
			if result.Error != nil || len(users) == 0 {
				break
			}

			for _, user := range users {
				data := map[string]interface{}{
					"id":         user.ID,
					"email":      user.Email,
					"name":       user.Name,
					"role":       user.Role,
					"active":     user.Active,
					"created_at": user.CreatedAt.Format(time.RFC3339),
					"updated_at": user.UpdatedAt.Format(time.RFC3339),
				}
				jsonBytes, _ := json.Marshal(data)
				fmt.Fprintf(w, "%s\n", jsonBytes)
			}
			
			if len(users) < batchSize {
				break
			}
			offset += batchSize
		}
	}
}

// streamArticles streams article data in the specified format
func streamArticles(w io.Writer, db *gorm.DB, format string, c *gin.Context) {
	const batchSize = 1000
	offset := 0

	if format == "csv" {
		csvWriter := csv.NewWriter(w)
		// Write header
		csvWriter.Write([]string{"id", "slug", "title", "body", "author_id", "tags", "status", "published_at", "created_at"})
		csvWriter.Flush()

		for {
			var articles []articles.ArticleModel
			result := db.Preload("Tags").Limit(batchSize).Offset(offset).Find(&articles)
			if result.Error != nil || len(articles) == 0 {
				break
			}

			for _, article := range articles {
				tagNames := []string{}
				for _, tag := range article.Tags {
					tagNames = append(tagNames, tag.Name)
				}
				
				publishedAt := ""
				if article.PublishedAt != nil {
					publishedAt = article.PublishedAt.Format(time.RFC3339)
				}

				csvWriter.Write([]string{
					article.ID,
					article.Slug,
					article.Title,
					article.Body,
					article.AuthorID,
					strings.Join(tagNames, ","),
					article.Status,
					publishedAt,
					article.CreatedAt.Format(time.RFC3339),
				})
			}
			csvWriter.Flush()
			
			if len(articles) < batchSize {
				break
			}
			offset += batchSize
		}
	} else {
		// NDJSON format
		for {
			var articles []articles.ArticleModel
			result := db.Preload("Tags").Limit(batchSize).Offset(offset).Find(&articles)
			if result.Error != nil || len(articles) == 0 {
				break
			}

			for _, article := range articles {
				tagNames := []string{}
				for _, tag := range article.Tags {
					tagNames = append(tagNames, tag.Name)
				}

				data := map[string]interface{}{
					"id":        article.ID,
					"slug":      article.Slug,
					"title":     article.Title,
					"body":      article.Body,
					"author_id": article.AuthorID,
					"tags":      tagNames,
					"status":    article.Status,
					"created_at": article.CreatedAt.Format(time.RFC3339),
				}
				
				if article.PublishedAt != nil {
					data["published_at"] = article.PublishedAt.Format(time.RFC3339)
				}

				jsonBytes, _ := json.Marshal(data)
				fmt.Fprintf(w, "%s\n", jsonBytes)
			}
			
			if len(articles) < batchSize {
				break
			}
			offset += batchSize
		}
	}
}

// streamComments streams comment data in the specified format
func streamComments(w io.Writer, db *gorm.DB, format string, c *gin.Context) {
	const batchSize = 1000
	offset := 0

	if format == "csv" {
		csvWriter := csv.NewWriter(w)
		// Write header
		csvWriter.Write([]string{"id", "article_id", "user_id", "body", "created_at"})
		csvWriter.Flush()

		for {
			var comments []articles.CommentModel
			result := db.Limit(batchSize).Offset(offset).Find(&comments)
			if result.Error != nil || len(comments) == 0 {
				break
			}

			for _, comment := range comments {
				csvWriter.Write([]string{
					comment.ID,
					comment.ArticleID,
					comment.UserID,
					comment.Body,
					comment.CreatedAt.Format(time.RFC3339),
				})
			}
			csvWriter.Flush()
			
			if len(comments) < batchSize {
				break
			}
			offset += batchSize
		}
	} else {
		// NDJSON format
		for {
			var comments []articles.CommentModel
			result := db.Limit(batchSize).Offset(offset).Find(&comments)
			if result.Error != nil || len(comments) == 0 {
				break
			}

			for _, comment := range comments {
				data := map[string]interface{}{
					"id":         comment.ID,
					"article_id": comment.ArticleID,
					"user_id":    comment.UserID,
					"body":       comment.Body,
					"created_at": comment.CreatedAt.Format(time.RFC3339),
				}
				jsonBytes, _ := json.Marshal(data)
				fmt.Fprintf(w, "%s\n", jsonBytes)
			}
			
			if len(comments) < batchSize {
				break
			}
			offset += batchSize
		}
	}
}

// CreateExportRequest represents the request for async export
type CreateExportRequest struct {
	IdempotencyKey string   `json:"idempotency_key" binding:"required"`
	ResourceType   string   `json:"resource_type" binding:"required,oneof=users articles comments"`
	Format         string   `json:"format" binding:"required,oneof=csv ndjson"`
	Fields         []string `json:"fields,omitempty"`        // Optional field selection
	Filters        map[string]string `json:"filters,omitempty"` // Optional filters
}

// CreateExportResponse represents the response for async export creation
type CreateExportResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateExport godoc
// @Summary Create async export job
// @Description Creates an export job to export filtered data asynchronously with download URL
// @Tags exports
// @Accept json
// @Produce json
// @Param export body CreateExportRequest true "Export configuration"
// @Success 202 {object} CreateExportResponse "Export job created"
// @Success 200 {object} CreateExportResponse "Existing job returned (idempotency)"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /exports [post]
func CreateExport(c *gin.Context) {
	var req CreateExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Check idempotency
	var existingJob common.ExportJob
	result := common.GetDB().Where("idempotency_key = ?", req.IdempotencyKey).First(&existingJob)
	if result.Error == nil {
		c.JSON(200, CreateExportResponse{
			JobID:     existingJob.ID,
			Status:    existingJob.Status,
			CreatedAt: existingJob.CreatedAt,
		})
		return
	}

	// Marshal filters to JSON
	filtersJSON, _ := json.Marshal(req.Filters)

	// Create export job
	job := common.ExportJob{
		ID:             uuid.New().String(),
		IdempotencyKey: req.IdempotencyKey,
		ResourceType:   req.ResourceType,
		Format:         req.Format,
		Filters:        string(filtersJSON),
		Status:         "pending",
		CreatedAt:      time.Now(),
	}

	if err := common.GetDB().Create(&job).Error; err != nil {
		c.JSON(500, gin.H{"error": "Failed to create export job"})
		return
	}

	// Start async export processing
	go ProcessExportJob(job.ID, req.Fields, req.Filters)

	c.JSON(202, CreateExportResponse{
		JobID:     job.ID,
		Status:    job.Status,
		CreatedAt: job.CreatedAt,
	})
}

// GetExport godoc
// @Summary Get export job status
// @Description Retrieves the status and download URL of an export job
// @Tags exports
// @Produce json
// @Param job_id path string true "Export Job ID"
// @Success 200 {object} map[string]interface{} "Export job details with download URL"
// @Failure 404 {object} map[string]string "Job not found"
// @Router /exports/{job_id} [get]
func GetExport(c *gin.Context) {
	jobID := c.Param("job_id")

	var job common.ExportJob
	if err := common.GetDB().Where("id = ?", jobID).First(&job).Error; err != nil {
		c.JSON(404, gin.H{"error": "Export job not found"})
		return
	}

	// Set rows processed for metrics
	c.Set("rows_processed", job.TotalRecords)

	response := gin.H{
		"job_id":        job.ID,
		"resource_type": job.ResourceType,
		"format":        job.Format,
		"status":        job.Status,
		"total_records": job.TotalRecords,
		"created_at":    job.CreatedAt,
	}

	if job.CompletedAt != nil {
		response["completed_at"] = job.CompletedAt
	}

	if job.DownloadURL != "" {
		response["download_url"] = job.DownloadURL
	}

	c.JSON(200, response)
}

// ProcessExportJob processes an export job in the background
func ProcessExportJob(jobID string, fields []string, filters map[string]string) {
	db := common.GetDB()

	// Fetch job
	var job common.ExportJob
	if err := db.Where("id = ?", jobID).First(&job).Error; err != nil {
		return
	}

	// Update status to processing
	job.Status = "processing"
	db.Save(&job)

	// Create export file
	exportDir := "./data/exports"
	os.MkdirAll(exportDir, 0750)
	
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s_%s.%s", job.ResourceType, jobID[:8], timestamp, job.Format)
	filepath := filepath.Join(exportDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		job.Status = "failed"
		now := time.Now()
		job.CompletedAt = &now
		db.Save(&job)
		return
	}
	defer file.Close()

	// Export data
	var exportErr error
	switch job.ResourceType {
	case "users":
		exportErr = exportUsers(file, db, job.Format, fields, filters, &job)
	case "articles":
		exportErr = exportArticles(file, db, job.Format, fields, filters, &job)
	case "comments":
		exportErr = exportComments(file, db, job.Format, fields, filters, &job)
	}

	if exportErr != nil {
		job.Status = "failed"
	} else {
		job.Status = "completed"
		job.DownloadURL = fmt.Sprintf("/exports/%s", filename)
	}

	now := time.Now()
	job.CompletedAt = &now
	db.Save(&job)
}

func exportUsers(file *os.File, db *gorm.DB, format string, fields []string, filters map[string]string, job *common.ExportJob) error {
	const batchSize = 1000
	offset := 0

	if format == "csv" {
		csvWriter := csv.NewWriter(file)
		csvWriter.Write([]string{"id", "email", "name", "role", "active", "created_at", "updated_at"})
		csvWriter.Flush()

		for {
			var users []users.UserModel
			query := db.Limit(batchSize).Offset(offset)
			
			// Apply filters
			if role, ok := filters["role"]; ok {
				query = query.Where("role = ?", role)
			}
			if active, ok := filters["active"]; ok {
				query = query.Where("active = ?", active == "true")
			}
			
			result := query.Find(&users)
			if result.Error != nil || len(users) == 0 {
				break
			}

			for _, user := range users {
				csvWriter.Write([]string{
					user.ID,
					user.Email,
					user.Name,
					user.Role,
					fmt.Sprintf("%t", user.Active),
					user.CreatedAt.Format(time.RFC3339),
					user.UpdatedAt.Format(time.RFC3339),
				})
				job.TotalRecords++
			}
			csvWriter.Flush()
			
			// Update progress periodically
			if offset%5000 == 0 {
				db.Model(job).Update("total_records", job.TotalRecords)
			}
			
			if len(users) < batchSize {
				break
			}
			offset += batchSize
		}
	} else {
		// NDJSON
		for {
			var users []users.UserModel
			query := db.Limit(batchSize).Offset(offset)
			
			// Apply filters
			if role, ok := filters["role"]; ok {
				query = query.Where("role = ?", role)
			}
			if active, ok := filters["active"]; ok {
				query = query.Where("active = ?", active == "true")
			}
			
			result := query.Find(&users)
			if result.Error != nil || len(users) == 0 {
				break
			}

			for _, user := range users {
				data := map[string]interface{}{
					"id":         user.ID,
					"email":      user.Email,
					"name":       user.Name,
					"role":       user.Role,
					"active":     user.Active,
					"created_at": user.CreatedAt.Format(time.RFC3339),
					"updated_at": user.UpdatedAt.Format(time.RFC3339),
				}
				jsonBytes, _ := json.Marshal(data)
				fmt.Fprintf(file, "%s\n", jsonBytes)
				job.TotalRecords++
			}
			
			// Update progress periodically
			if offset%5000 == 0 {
				db.Model(job).Update("total_records", job.TotalRecords)
			}
			
			if len(users) < batchSize {
				break
			}
			offset += batchSize
		}
	}

	return nil
}

func exportArticles(file *os.File, db *gorm.DB, format string, fields []string, filters map[string]string, job *common.ExportJob) error {
	const batchSize = 1000
	offset := 0

	if format == "csv" {
		csvWriter := csv.NewWriter(file)
		csvWriter.Write([]string{"id", "slug", "title", "body", "author_id", "tags", "status", "published_at", "created_at"})
		csvWriter.Flush()

		for {
			var articles []articles.ArticleModel
			query := db.Preload("Tags").Limit(batchSize).Offset(offset)
			
			// Apply filters
			if status, ok := filters["status"]; ok {
				query = query.Where("status = ?", status)
			}
			if authorID, ok := filters["author_id"]; ok {
				query = query.Where("author_id = ?", authorID)
			}
			
			result := query.Find(&articles)
			if result.Error != nil || len(articles) == 0 {
				break
			}

			for _, article := range articles {
				tagNames := []string{}
				for _, tag := range article.Tags {
					tagNames = append(tagNames, tag.Name)
				}
				
				publishedAt := ""
				if article.PublishedAt != nil {
					publishedAt = article.PublishedAt.Format(time.RFC3339)
				}

				csvWriter.Write([]string{
					article.ID,
					article.Slug,
					article.Title,
					article.Body,
					article.AuthorID,
					strings.Join(tagNames, ","),
					article.Status,
					publishedAt,
					article.CreatedAt.Format(time.RFC3339),
				})
				job.TotalRecords++
			}
			csvWriter.Flush()
			
			if offset%5000 == 0 {
				db.Model(job).Update("total_records", job.TotalRecords)
			}
			
			if len(articles) < batchSize {
				break
			}
			offset += batchSize
		}
	} else {
		// NDJSON
		for {
			var articles []articles.ArticleModel
			query := db.Preload("Tags").Limit(batchSize).Offset(offset)
			
			// Apply filters
			if status, ok := filters["status"]; ok {
				query = query.Where("status = ?", status)
			}
			if authorID, ok := filters["author_id"]; ok {
				query = query.Where("author_id = ?", authorID)
			}
			
			result := query.Find(&articles)
			if result.Error != nil || len(articles) == 0 {
				break
			}

			for _, article := range articles {
				tagNames := []string{}
				for _, tag := range article.Tags {
					tagNames = append(tagNames, tag.Name)
				}

				data := map[string]interface{}{
					"id":        article.ID,
					"slug":      article.Slug,
					"title":     article.Title,
					"body":      article.Body,
					"author_id": article.AuthorID,
					"tags":      tagNames,
					"status":    article.Status,
					"created_at": article.CreatedAt.Format(time.RFC3339),
				}
				
				if article.PublishedAt != nil {
					data["published_at"] = article.PublishedAt.Format(time.RFC3339)
				}

				jsonBytes, _ := json.Marshal(data)
				fmt.Fprintf(file, "%s\n", jsonBytes)
				job.TotalRecords++
			}
			
			if offset%5000 == 0 {
				db.Model(job).Update("total_records", job.TotalRecords)
			}
			
			if len(articles) < batchSize {
				break
			}
			offset += batchSize
		}
	}

	return nil
}

func exportComments(file *os.File, db *gorm.DB, format string, fields []string, filters map[string]string, job *common.ExportJob) error {
	const batchSize = 1000
	offset := 0

	if format == "csv" {
		csvWriter := csv.NewWriter(file)
		csvWriter.Write([]string{"id", "article_id", "user_id", "body", "created_at"})
		csvWriter.Flush()

		for {
			var comments []articles.CommentModel
			query := db.Limit(batchSize).Offset(offset)
			
			// Apply filters
			if articleID, ok := filters["article_id"]; ok {
				query = query.Where("article_id = ?", articleID)
			}
			if userID, ok := filters["user_id"]; ok {
				query = query.Where("user_id = ?", userID)
			}
			
			result := query.Find(&comments)
			if result.Error != nil || len(comments) == 0 {
				break
			}

			for _, comment := range comments {
				csvWriter.Write([]string{
					comment.ID,
					comment.ArticleID,
					comment.UserID,
					comment.Body,
					comment.CreatedAt.Format(time.RFC3339),
				})
				job.TotalRecords++
			}
			csvWriter.Flush()
			
			if offset%5000 == 0 {
				db.Model(job).Update("total_records", job.TotalRecords)
			}
			
			if len(comments) < batchSize {
				break
			}
			offset += batchSize
		}
	} else {
		// NDJSON
		for {
			var comments []articles.CommentModel
			query := db.Limit(batchSize).Offset(offset)
			
			// Apply filters
			if articleID, ok := filters["article_id"]; ok {
				query = query.Where("article_id = ?", articleID)
			}
			if userID, ok := filters["user_id"]; ok {
				query = query.Where("user_id = ?", userID)
			}
			
			result := query.Find(&comments)
			if result.Error != nil || len(comments) == 0 {
				break
			}

			for _, comment := range comments {
				data := map[string]interface{}{
					"id":         comment.ID,
					"article_id": comment.ArticleID,
					"user_id":    comment.UserID,
					"body":       comment.Body,
					"created_at": comment.CreatedAt.Format(time.RFC3339),
				}
				jsonBytes, _ := json.Marshal(data)
				fmt.Fprintf(file, "%s\n", jsonBytes)
				job.TotalRecords++
			}
			
			if offset%5000 == 0 {
				db.Model(job).Update("total_records", job.TotalRecords)
			}
			
			if len(comments) < batchSize {
				break
			}
			offset += batchSize
		}
	}

	return nil
}
