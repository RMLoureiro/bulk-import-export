package users

import (
	"strings"
	"time"

	"bulk-import-export/common"

	"github.com/google/uuid"
)

// UserValidator validates user records for bulk import
type UserValidator struct {
	existingIDs    map[string]bool
	existingEmails map[string]bool
}

// NewUserValidator creates a validator with pre-loaded existing data
func NewUserValidator() *UserValidator {
	validator := &UserValidator{
		existingIDs:    make(map[string]bool),
		existingEmails: make(map[string]bool),
	}

	// Don't pre-load existing data - let database handle uniqueness via upsert
	// This significantly speeds up validation initialization for large datasets

	return validator
}

// ValidateUserRecord validates a single user record from import
func (v *UserValidator) ValidateUserRecord(record map[string]string, rowNum int) *common.RecordValidationResult {
	result := &common.RecordValidationResult{
		RowNumber: rowNum,
		RecordID:  record["id"],
		Valid:     true,
	}

	// Validate email (required - natural key for upsert per spec)
	email := strings.TrimSpace(record["email"])

	if email == "" {
		result.AddError("email", "Email is required (natural key for upsert)")
	} else if !common.ValidateEmail(email) {
		result.AddError("email", "Invalid email format")
	}

	// Validate role (required)
	role := strings.TrimSpace(record["role"])
	if role == "" {
		result.AddError("role", "Role is required")
	}

	// Validate active (required, boolean)
	active := strings.ToLower(strings.TrimSpace(record["active"]))
	if active != "true" && active != "false" {
		result.AddError("active", "Active must be 'true' or 'false'")
	}

	return result
}

// NormalizeUserRecord normalizes and fills defaults for a user record
func NormalizeUserRecord(record map[string]string) UserModel {
	now := time.Now()

	// Generate ID if not provided (natural key upsert by email)
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
