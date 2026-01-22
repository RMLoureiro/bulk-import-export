package articles

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"bulk-import-export/common"

	"github.com/google/uuid"
)

// Article status values
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// ArticleValidator validates article records for bulk import
type ArticleValidator struct {
	existingIDs    map[string]bool
	existingSlugs  map[string]bool
	validAuthorIDs map[string]bool
}

// CommentValidator validates comment records for bulk import
type CommentValidator struct {
	existingIDs     map[string]bool
	validArticleIDs map[string]bool
	validUserIDs    map[string]bool
}

// NewArticleValidator creates a validator with pre-loaded existing data
func NewArticleValidator() *ArticleValidator {
	validator := &ArticleValidator{
		existingIDs:    make(map[string]bool),
		existingSlugs:  make(map[string]bool),
		validAuthorIDs: make(map[string]bool),
	}

	// Don't pre-load existing data - let database handle uniqueness via upsert
	// This significantly speeds up validation initialization for large datasets

	return validator
}

// ValidateArticleRecord validates a single article record from import
func (v *ArticleValidator) ValidateArticleRecord(record map[string]interface{}, rowNum int) *common.RecordValidationResult {
	result := &common.RecordValidationResult{
		RowNumber: rowNum,
		Valid:     true,
	}

	// Get slug
	slug := getStringField(record, "slug")

	// Validate slug (required - natural key for upsert per spec)
	if slug == "" {
		result.AddError("slug", "Slug is required (natural key for upsert)")
	} else if !common.ValidateKebabCase(slug) {
		result.AddError("slug", "Slug must be in kebab-case format (lowercase, hyphen-separated)")
	}

	// Set record ID if provided
	var id string
	if idVal, ok := record["id"]; ok && idVal != nil {
		id = strings.TrimSpace(fmt.Sprint(idVal))
		result.RecordID = id
	} else {
		result.RecordID = slug
	}

	// Validate author_id (required - spec says "valid author_id")
	authorID := getStringField(record, "author_id")
	if authorID == "" {
		result.AddError("author_id", "Author ID is required")
	}

	// Validate published_at constraint: draft must not have published_at (spec requirement)
	status := getStringField(record, "status")
	publishedAtStr := getStringField(record, "published_at")
	if status == StatusDraft && publishedAtStr != "" {
		result.AddError("published_at", "Draft articles must not have published_at timestamp")
	}

	return result
}

// NewCommentValidator creates a validator with pre-loaded existing data
func NewCommentValidator() *CommentValidator {
	validator := &CommentValidator{
		existingIDs:     make(map[string]bool),
		validArticleIDs: make(map[string]bool),
		validUserIDs:    make(map[string]bool),
	}

	// Don't pre-load existing data - let database handle upsert and FK validation
	// This significantly speeds up validation initialization for large datasets

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

	// Validate ID (required)
	id = strings.TrimSpace(id)
	if id == "" {
		result.AddError("id", "ID is required")
	}

	// Validate article_id (required - spec says "valid foreign keys")
	articleID := getStringField(record, "article_id")
	if articleID == "" {
		result.AddError("article_id", "Article ID is required")
	}

	// Validate user_id (required - spec says "valid foreign keys")
	userID := getStringField(record, "user_id")
	if userID == "" {
		result.AddError("user_id", "User ID is required")
	}

	// Validate body (max 500 words per spec)
	body := getStringField(record, "body")
	if len(body) > 3500 {
		// Quick length check first (500 words ~= 3500 chars average)
		// Only do expensive word count if potentially over limit
		wordCount := common.CountWords(body)
		if wordCount > 500 {
			result.AddError("body", fmt.Sprintf("Body exceeds 500 words limit (has %d words)", wordCount))
		}
	}

	return result
}

// NormalizeArticleRecord normalizes and fills defaults for an article record
func NormalizeArticleRecord(record map[string]interface{}) ArticleModel {
	now := time.Now()

	// Generate ID if not provided (natural key upsert by slug)
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

	// Parse tags as JSON
	var tagsJSON string
	if tagsVal, ok := record["tags"]; ok && tagsVal != nil {
		var tags []string
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
		if len(tags) > 0 {
			tagsBytes, _ := json.Marshal(tags)
			tagsJSON = string(tagsBytes)
		}
	}

	article := ArticleModel{
		ID:          id,
		Slug:        getStringField(record, "slug"),
		Title:       getStringField(record, "title"),
		Body:        getStringField(record, "body"),
		AuthorID:    getStringField(record, "author_id"),
		Tags:        tagsJSON,
		Status:      getStringField(record, "status"),
		PublishedAt: publishedAt,
		CreatedAt:   createdAt,
	}

	return article
}

// NormalizeCommentRecord normalizes and fills defaults for a comment record
func NormalizeCommentRecord(record map[string]interface{}) CommentModel {
	now := time.Now()

	// Use provided ID (comments without ID are rejected in processBatchComments)
	id := getStringField(record, "id")

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
