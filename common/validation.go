package common

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ValidationError represents a single validation error for a record
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// RecordValidationResult holds validation results for a single record
type RecordValidationResult struct {
	RowNumber int               `json:"row_number"`
	RecordID  string            `json:"record_id,omitempty"`
	Valid     bool              `json:"valid"`
	Errors    []ValidationError `json:"errors,omitempty"`
}

// AddError adds a validation error to the result
func (r *RecordValidationResult) AddError(field, message string) {
	r.Valid = false
	r.Errors = append(r.Errors, ValidationError{
		Field:   field,
		Message: message,
	})
}

// ToJSON converts validation errors to JSON string
func (r *RecordValidationResult) ToJSON() string {
	if len(r.Errors) == 0 {
		return ""
	}
	data, _ := json.Marshal(r.Errors)
	return string(data)
}

// Email validation regex (simplified RFC 5322)
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail checks if email format is valid
func ValidateEmail(email string) bool {
	if email == "" {
		return false
	}
	return emailRegex.MatchString(email)
}

// Kebab-case validation regex
var kebabRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateKebabCase checks if string is in kebab-case format
func ValidateKebabCase(s string) bool {
	if s == "" {
		return false
	}
	return kebabRegex.MatchString(s)
}

// ValidateRequired checks if a string field is not empty
func ValidateRequired(field, value string) *ValidationError {
	if strings.TrimSpace(value) == "" {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("%s is required", field),
		}
	}
	return nil
}

// ValidateEnum checks if value is in allowed list
func ValidateEnum(field, value string, allowed []string) *ValidationError {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return &ValidationError{
		Field:   field,
		Message: fmt.Sprintf("%s must be one of: %s", field, strings.Join(allowed, ", ")),
	}
}

// CountWords counts words in a string (simple space-based)
func CountWords(s string) int {
	if s == "" {
		return 0
	}
	words := strings.Fields(s)
	return len(words)
}
