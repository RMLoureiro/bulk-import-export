package users

import (
	"strings"
	"time"

	"bulk-import-export/common"
	"github.com/google/uuid"
)

// UserValidator validates user records for bulk import
type UserValidator struct {
	existingIDs   map[string]bool
	existingEmails map[string]bool
}

// NewUserValidator creates a validator with pre-loaded existing data
func NewUserValidator() *UserValidator {
	db := common.GetDB()
	
	validator := &UserValidator{
		existingIDs:   make(map[string]bool),
		existingEmails: make(map[string]bool),
	}
	
	// Pre-load existing IDs and emails for validation
	var users []UserModel
	db.Select("id, email").Find(&users)
	
	for _, user := range users {
		validator.existingIDs[user.ID] = true
		validator.existingEmails[strings.ToLower(user.Email)] = true
	}
	
	return validator
}

// ValidateUserRecord validates a single user record from import
func (v *UserValidator) ValidateUserRecord(record map[string]string, rowNum int) *common.RecordValidationResult {
	result := &common.RecordValidationResult{
		RowNumber: rowNum,
		RecordID:  record["id"],
		Valid:     true,
	}
	
	// Validate ID (optional - will be generated if empty)
	id := strings.TrimSpace(record["id"])
	if id != "" {
		// If provided, must be valid UUID
		if _, err := uuid.Parse(id); err != nil {
			result.AddError("id", "Invalid UUID format")
		}
	}
	
	// Validate email (required)
	email := strings.TrimSpace(record["email"])
	if email == "" {
		result.AddError("email", "Email is required")
	} else {
		if !common.ValidateEmail(email) {
			result.AddError("email", "Invalid email format")
		}
		// Check uniqueness (case-insensitive)
		emailLower := strings.ToLower(email)
		if v.existingEmails[emailLower] {
			result.AddError("email", "Email already exists")
		}
	}
	
	// Validate name (required)
	name := strings.TrimSpace(record["name"])
	if name == "" {
		result.AddError("name", "Name is required")
	}
	
	// Validate role (required, enum)
	role := strings.TrimSpace(record["role"])
	allowedRoles := []string{"admin", "author", "reader", "manager"}
	if err := common.ValidateEnum("role", role, allowedRoles); err != nil {
		result.AddError(err.Field, err.Message)
	}
	
	// Validate active (required, boolean)
	active := strings.ToLower(strings.TrimSpace(record["active"]))
	if active != "true" && active != "false" {
		result.AddError("active", "Active must be 'true' or 'false'")
	}
	
	// Validate created_at (optional - will use current time if empty)
	if createdAtStr := strings.TrimSpace(record["created_at"]); createdAtStr != "" {
		if _, err := time.Parse(time.RFC3339, createdAtStr); err != nil {
			result.AddError("created_at", "Invalid timestamp format (use RFC3339)")
		}
	}
	
	// Validate updated_at (optional - will use current time if empty)
	if updatedAtStr := strings.TrimSpace(record["updated_at"]); updatedAtStr != "" {
		if _, err := time.Parse(time.RFC3339, updatedAtStr); err != nil {
			result.AddError("updated_at", "Invalid timestamp format (use RFC3339)")
		}
	}
	
	// Track this email for subsequent validations in same batch
	if result.Valid && email != "" {
		v.existingEmails[strings.ToLower(email)] = true
	}
	
	return result
}

// NormalizeUserRecord normalizes and fills defaults for a user record
func NormalizeUserRecord(record map[string]string) UserModel {
	now := time.Now()
	
	// Generate ID if not provided
	id := strings.TrimSpace(record["id"])
	if id == "" {
		id = uuid.New().String()
	}
	
	// Parse timestamps
	createdAt := now
	if createdAtStr := strings.TrimSpace(record["created_at"]); createdAtStr != "" {
		if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			createdAt = t
		}
	}
	
	updatedAt := now
	if updatedAtStr := strings.TrimSpace(record["updated_at"]); updatedAtStr != "" {
		if t, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
			updatedAt = t
		}
	}
	
	// Parse active boolean
	active := strings.ToLower(strings.TrimSpace(record["active"])) == "true"
	
	return UserModel{
		ID:        id,
		Email:     strings.TrimSpace(record["email"]),
		Name:      strings.TrimSpace(record["name"]),
		Role:      strings.TrimSpace(record["role"]),
		Active:    active,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

// BatchValidateUsers validates multiple user records efficiently
func BatchValidateUsers(records []map[string]string, startRow int) []*common.RecordValidationResult {
	validator := NewUserValidator()
	results := make([]*common.RecordValidationResult, len(records))
	
	for i, record := range records {
		results[i] = validator.ValidateUserRecord(record, startRow+i)
	}
	
	return results
}
