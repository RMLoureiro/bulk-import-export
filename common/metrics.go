package common

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// MetricsMiddleware tracks API performance metrics
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate request ID for tracing
		requestID := uuid.New().String()
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		// Record start time
		startTime := time.Now()

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(startTime)
		durationMs := int(duration.Milliseconds())

		// Get rows processed (if set by handler)
		rowsProcessed := 0
		if rows, exists := c.Get("rows_processed"); exists {
			if r, ok := rows.(int); ok {
				rowsProcessed = r
			}
		}

		// Get errors (if any)
		errors := ""
		if len(c.Errors) > 0 {
			errors = c.Errors.JSON().(string)
		}

		// Create metric record
		metric := ApiMetric{
			Endpoint:      c.FullPath(),
			Method:        c.Request.Method,
			StatusCode:    c.Writer.Status(),
			DurationMs:    durationMs,
			RowsProcessed: rowsProcessed,
			Errors:        errors,
			Timestamp:     startTime,
		}

		// Save metric asynchronously
		go func() {
			db := GetDB()
			db.Create(&metric)
		}()
	}
}
