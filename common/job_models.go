package common

import (
	"time"

	"gorm.io/gorm"
)

// ImportJob tracks the status of import operations
type ImportJob struct {
	ID             string     `gorm:"primaryKey;type:text" json:"id"`
	IdempotencyKey string     `gorm:"uniqueIndex;not null" json:"idempotency_key"`
	ResourceType   string     `gorm:"not null" json:"resource_type"` // users, articles, comments
	Status         string     `gorm:"not null" json:"status"`        // pending, processing, completed, failed
	FilePath       string     `json:"file_path,omitempty"`
	TotalRecords   int        `gorm:"default:0" json:"total_records"`
	ProcessedCount int        `gorm:"default:0" json:"processed_count"`
	SuccessCount   int        `gorm:"default:0" json:"success_count"`
	FailCount      int        `gorm:"default:0" json:"fail_count"`
	Errors         string     `gorm:"type:text" json:"errors,omitempty"` // JSON array of errors
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null" json:"updated_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

// ExportJob tracks the status of export operations
type ExportJob struct {
	ID             string     `gorm:"primaryKey;type:text" json:"id"`
	IdempotencyKey string     `gorm:"uniqueIndex;not null" json:"idempotency_key"`
	ResourceType   string     `gorm:"not null" json:"resource_type"`
	Format         string     `gorm:"not null" json:"format"` // csv, ndjson
	Filters        string     `gorm:"type:text" json:"filters,omitempty"` // JSON filters
	Status         string     `gorm:"not null" json:"status"`
	FilePath       string     `json:"file_path,omitempty"`
	DownloadURL    string     `json:"download_url,omitempty"`
	TotalRecords   int        `gorm:"default:0" json:"total_records"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

// ApiMetric tracks API performance metrics
type ApiMetric struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Endpoint      string    `gorm:"not null" json:"endpoint"`
	Method        string    `gorm:"not null" json:"method"`
	StatusCode    int       `gorm:"not null" json:"status_code"`
	DurationMs    int       `gorm:"not null" json:"duration_ms"`
	RowsProcessed int       `gorm:"default:0" json:"rows_processed"`
	Errors        string    `gorm:"type:text" json:"errors,omitempty"` // JSON errors
	Timestamp     time.Time `gorm:"not null" json:"timestamp"`
}

func (ImportJob) TableName() string { return "import_jobs" }
func (ExportJob) TableName() string { return "export_jobs" }
func (ApiMetric) TableName() string { return "api_metrics" }

// AutoMigrateJobs creates job tracking tables
func AutoMigrateJobs(db *gorm.DB) {
	db.AutoMigrate(&ImportJob{}, &ExportJob{}, &ApiMetric{})
}
