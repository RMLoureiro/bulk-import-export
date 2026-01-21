package articles

import (
	"fmt"
	"strings"
	"time"

	"bulk-import-export/common"
	"github.com/google/uuid"
)

// ArticleValidator validates article records for bulk import
type ArticleValidator struct {
	existingIDs     map[string]bool
	existingSlugs   map[string]bool
	validAuthorIDs  map[string]bool
}

// CommentValidator validates comment records for bulk import
type CommentValidator struct {
	existingIDs      map[string]bool
	validArticleIDs  map[string]bool
	validUserIDs     map[string]bool
}

// NewArticleValidator creates a validator with pre-loaded existing data
func NewArticleValidator() *ArticleValidator {
	db := common.GetDB()
	
	validator := &ArticleValidator{
		existingIDs:    make(map[string]bool),
		existingSlugs:  make(map[string]bool),
		validAuthorIDs: make(map[string]bool),
	}
	
	// Pre-load existing article IDs and slugs
	var articles []ArticleModel
	db.Select("id, slug").Find(&articles)
	for _, article := range articles {
		validator.existingIDs[article.ID] = true
		validator.existingSlugs[article.Slug] = true
	}
	
	// Pre-load valid author IDs (all user IDs)
	var userIDs []string
	db.Table("users").Select("id").Find(&userIDs)
	for _, id := range userIDs {
		validator.validAuthorIDs[id] = true
	}
	
	return validator
}

// ValidateArticleRecord validates a single article record from import
func (v *ArticleValidator) ValidateArticleRecord(record map[string]interface{}, rowNum int) *common.RecordValidationResult {
	result := &common.RecordValidationResult{
		RowNumber: rowNum,
		Valid:     true,
	}
	
	// Get ID as string
	var id string
	if idVal, ok := record["id"]; ok && idVal != nil {
		id = fmt.Sprint(idVal)
		result.RecordID = id
	}
	
	// Validate ID (optional - will be generated if empty)
	id = strings.TrimSpace(id)
	if id != "" {
		if _, err := uuid.Parse(id); err != nil {
			result.AddError("id", "Invalid UUID format")
		}
	}
	
	// Validate slug (required, kebab-case)
	slug := getStringField(record, "slug")
	if slug == "" {
		result.AddError("slug", "Slug is required")
	} else {
		if !common.ValidateKebabCase(slug) {
			result.AddError("slug", "Slug must be in kebab-case format (lowercase, hyphen-separated)")
		}
		// Check uniqueness
		if v.existingSlugs[slug] {
			result.AddError("slug", "Slug already exists")
		}
	}
	
	// Validate title (required)
	title := getStringField(record, "title")
	if title == "" {
		result.AddError("title", "Title is required")
	}
	
	// Validate body (required)
	body := getStringField(record, "body")
	if body == "" {
		result.AddError("body", "Body is required")
	}
	
	// Validate author_id (required, FK)
	authorID := getStringField(record, "author_id")
	if authorID == "" {
		result.AddError("author_id", "Author ID is required")
	} else {
		if _, err := uuid.Parse(authorID); err != nil {
			result.AddError("author_id", "Invalid author ID UUID format")
		} else if !v.validAuthorIDs[authorID] {
			result.AddError("author_id", "Author ID does not exist in users table")
		}
	}
	
	// Validate status (required, enum)
	status := getStringField(record, "status")
	allowedStatuses := []string{"draft", "published"}
	if err := common.ValidateEnum("status", status, allowedStatuses); err != nil {
		result.AddError(err.Field, err.Message)
	}
	
	// Validate published_at (optional, but required if status=published)
	publishedAtStr := getStringField(record, "published_at")
	if status == "published" && publishedAtStr == "" {
		result.AddError("published_at", "Published articles must have published_at timestamp")
	}
	if status == "draft" && publishedAtStr != "" {
		result.AddError("published_at", "Draft articles must not have published_at timestamp")
	}
	if publishedAtStr != "" {
		if _, err := time.Parse(time.RFC3339, publishedAtStr); err != nil {
			result.AddError("published_at", "Invalid timestamp format (use RFC3339)")
		}
	}
	
	// Validate created_at (optional)
	if createdAtStr := getStringField(record, "created_at"); createdAtStr != "" {
		if _, err := time.Parse(time.RFC3339, createdAtStr); err != nil {
			result.AddError("created_at", "Invalid timestamp format (use RFC3339)")
		}
	}
	
	// Validate tags (optional, array)
	if tagsVal, ok := record["tags"]; ok && tagsVal != nil {
		// Tags can be JSON array or comma-separated string
		switch v := tagsVal.(type) {
		case []interface{}:
			// Valid array
		case string:
			// Valid comma-separated string
		default:
			result.AddError("tags", fmt.Sprintf("Tags must be array or string, got %T", v))
		}
	}
	
	// Track this slug for subsequent validations in same batch
	if result.Valid && slug != "" {
		v.existingSlugs[slug] = true
	}
	
	return result
}

// NewCommentValidator creates a validator with pre-loaded existing data
func NewCommentValidator() *CommentValidator {
	db := common.GetDB()
	
	validator := &CommentValidator{
		existingIDs:     make(map[string]bool),
		validArticleIDs: make(map[string]bool),
		validUserIDs:    make(map[string]bool),
	}
	
	// Pre-load existing comment IDs
	var commentIDs []string
	db.Table("comments").Select("id").Find(&commentIDs)
	for _, id := range commentIDs {
		validator.existingIDs[id] = true
	}
	
	// Pre-load valid article IDs
	var articleIDs []string
	db.Table("articles").Select("id").Find(&articleIDs)
	for _, id := range articleIDs {
		validator.validArticleIDs[id] = true
	}
	
	// Pre-load valid user IDs
	var userIDs []string
	db.Table("users").Select("id").Find(&userIDs)
	for _, id := range userIDs {
		validator.validUserIDs[id] = true
	}
	
	return validator
}

// ValidateCommentRecord validates a single comment record from import
func (v *CommentValidator) ValidateCommentRecord(record map[string]interface{}, rowNum int) *common.RecordValidationResult {
	result := &common.RecordValidationResult{
		RowNumber: rowNum,
		Valid:     true,
	}
	
	// Get ID as string
	var id string
	if idVal, ok := record["id"]; ok && idVal != nil {
		id = fmt.Sprint(idVal)
		result.RecordID = id
	}
	
	// Validate ID (optional - will be generated if empty)
	id = strings.TrimSpace(id)
	if id != "" {
		if _, err := uuid.Parse(id); err != nil {
			result.AddError("id", "Invalid UUID format")
		}
	}
	
	// Validate article_id (required, FK)
	articleID := getStringField(record, "article_id")
	if articleID == "" {
		result.AddError("article_id", "Article ID is required")
	} else {
		if _, err := uuid.Parse(articleID); err != nil {
			result.AddError("article_id", "Invalid article ID UUID format")
		} else if !v.validArticleIDs[articleID] {
			result.AddError("article_id", "Article ID does not exist in articles table")
		}
	}
	
	// Validate user_id (required, FK)
	userID := getStringField(record, "user_id")
	if userID == "" {
		result.AddError("user_id", "User ID is required")
	} else {
		if _, err := uuid.Parse(userID); err != nil {
			result.AddError("user_id", "Invalid user ID UUID format")
		} else if !v.validUserIDs[userID] {
			result.AddError("user_id", "User ID does not exist in users table")
		}
	}
	
	// Validate body (required, max 500 words)
	body := getStringField(record, "body")
	if body == "" {
		result.AddError("body", "Body is required")
	} else {
		wordCount := common.CountWords(body)
		if wordCount > 500 {
			result.AddError("body", fmt.Sprintf("Body exceeds 500 words limit (has %d words)", wordCount))
		}
	}
	
	// Validate created_at (optional)
	if createdAtStr := getStringField(record, "created_at"); createdAtStr != "" {
		if _, err := time.Parse(time.RFC3339, createdAtStr); err != nil {
			result.AddError("created_at", "Invalid timestamp format (use RFC3339)")
		}
	}
	
	return result
}

// NormalizeArticleRecord normalizes and fills defaults for an article record
func NormalizeArticleRecord(record map[string]interface{}) (ArticleModel, []string) {
	now := time.Now()
	
	// Generate ID if not provided
	id := getStringField(record, "id")
	if id == "" {
		id = uuid.New().String()
	}
	
	// Parse timestamps
	createdAt := now
	if createdAtStr := getStringField(record, "created_at"); createdAtStr != "" {
		if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			createdAt = t
		}
	}
	
	var publishedAt *time.Time
	if publishedAtStr := getStringField(record, "published_at"); publishedAtStr != "" {
		if t, err := time.Parse(time.RFC3339, publishedAtStr); err == nil {
			publishedAt = &t
		}
	}
	
	// Parse tags
	var tags []string
	if tagsVal, ok := record["tags"]; ok && tagsVal != nil {
		switch v := tagsVal.(type) {
		case []interface{}:
			for _, tag := range v {
				tags = append(tags, fmt.Sprint(tag))
			}
		case string:
			if v != "" {
				tags = strings.Split(v, ",")
				for i := range tags {
					tags[i] = strings.TrimSpace(tags[i])
				}
			}
		}
	}
	
	article := ArticleModel{
		ID:          id,
		Slug:        getStringField(record, "slug"),
		Title:       getStringField(record, "title"),
		Body:        getStringField(record, "body"),
		AuthorID:    getStringField(record, "author_id"),
		Status:      getStringField(record, "status"),
		PublishedAt: publishedAt,
		CreatedAt:   createdAt,
	}
	
	return article, tags
}

// NormalizeCommentRecord normalizes and fills defaults for a comment record
func NormalizeCommentRecord(record map[string]interface{}) CommentModel {
	now := time.Now()
	
	// Generate ID if not provided
	id := getStringField(record, "id")
	if id == "" {
		id = uuid.New().String()
	}
	
	// Parse timestamp
	createdAt := now
	if createdAtStr := getStringField(record, "created_at"); createdAtStr != "" {
		if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			createdAt = t
		}
	}
	
	return CommentModel{
		ID:        id,
		ArticleID: getStringField(record, "article_id"),
		UserID:    getStringField(record, "user_id"),
		Body:      getStringField(record, "body"),
		CreatedAt: createdAt,
	}
}

// Helper to extract string field from map[string]interface{}
func getStringField(record map[string]interface{}, field string) string {
	if val, ok := record[field]; ok && val != nil {
		return strings.TrimSpace(fmt.Sprint(val))
	}
	return ""
}

// BatchValidateArticles validates multiple article records efficiently
func BatchValidateArticles(records []map[string]interface{}, startRow int) []*common.RecordValidationResult {
	validator := NewArticleValidator()
	results := make([]*common.RecordValidationResult, len(records))
	
	for i, record := range records {
		results[i] = validator.ValidateArticleRecord(record, startRow+i)
	}
	
	return results
}

// BatchValidateComments validates multiple comment records efficiently
func BatchValidateComments(records []map[string]interface{}, startRow int) []*common.RecordValidationResult {
	validator := NewCommentValidator()
	results := make([]*common.RecordValidationResult, len(records))
	
	for i, record := range records {
		results[i] = validator.ValidateCommentRecord(record, startRow+i)
	}
	
	return results
}

// UpsertTags creates tags if they don't exist and returns tag IDs
func UpsertTags(tagNames []string) ([]TagModel, error) {
	if len(tagNames) == 0 {
		return []TagModel{}, nil
	}
	
	db := common.GetDB()
	var tags []TagModel
	
	for _, name := range tagNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		
		var tag TagModel
		result := db.Where("name = ?", name).FirstOrCreate(&tag, TagModel{Name: name})
		if result.Error != nil {
			return nil, result.Error
		}
		tags = append(tags, tag)
	}
	
	return tags, nil
}
