package common

import (
	"testing"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"user@example.com", true},
		{"test.user+tag@domain.co.uk", true},
		{"", false},
		{"invalid", false},
		{"@domain.com", false},
		{"user@", false},
		{"user @domain.com", false},
	}
	
	for _, tt := range tests {
		result := ValidateEmail(tt.email)
		if result != tt.valid {
			t.Errorf("ValidateEmail(%q) = %v, want %v", tt.email, result, tt.valid)
		}
	}
}

func TestValidateKebabCase(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"hello-world", true},
		{"my-article-slug", true},
		{"single", true},
		{"with-123-numbers", true},
		{"", false},
		{"Hello-World", false},
		{"hello_world", false},
		{"hello world", false},
		{"-starts-with-dash", false},
		{"ends-with-dash-", false},
		{"double--dash", false},
	}
	
	for _, tt := range tests {
		result := ValidateKebabCase(tt.input)
		if result != tt.valid {
			t.Errorf("ValidateKebabCase(%q) = %v, want %v", tt.input, result, tt.valid)
		}
	}
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		input string
		count int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  multiple   spaces   between  ", 3},
		{"word1 word2 word3 word4 word5", 5},
	}
	
	for _, tt := range tests {
		result := CountWords(tt.input)
		if result != tt.count {
			t.Errorf("CountWords(%q) = %d, want %d", tt.input, result, tt.count)
		}
	}
}

func TestValidationError(t *testing.T) {
	result := &RecordValidationResult{
		RowNumber: 1,
		RecordID:  "test-id",
		Valid:     true,
	}
	
	// Initially valid, no errors
	if !result.Valid || len(result.Errors) != 0 {
		t.Error("New result should be valid with no errors")
	}
	
	// Add error
	result.AddError("email", "Invalid email format")
	
	if result.Valid {
		t.Error("Result should be invalid after adding error")
	}
	
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
	
	if result.Errors[0].Field != "email" {
		t.Errorf("Expected field 'email', got %q", result.Errors[0].Field)
	}
	
	// Add multiple errors
	result.AddError("name", "Name is required")
	result.AddError("role", "Invalid role")
	
	if len(result.Errors) != 3 {
		t.Errorf("Expected 3 errors, got %d", len(result.Errors))
	}
	
	// Test JSON conversion
	json := result.ToJSON()
	if json == "" {
		t.Error("ToJSON should return non-empty string")
	}
}
