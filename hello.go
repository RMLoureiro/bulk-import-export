package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"bulk-import-export/articles"
	"bulk-import-export/common"
	"bulk-import-export/users"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	// Migrate domain models
	users.AutoMigrate()
	articles.AutoMigrate()

	// Migrate job tracking tables
	common.AutoMigrateJobs(db)
}

func main() {
	// Initialize database
	db := common.Init()
	Migrate(db)

	// Ensure database connection is closed on exit
	sqlDB, err := db.DB()
	if err != nil {
		log.Println("Failed to get sql.DB:", err)
	} else {
		defer sqlDB.Close()
	}

	// Setup Gin router
	r := gin.Default()
	r.RedirectTrailingSlash = false

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// TODO: Will add import/export routes in next steps
	// v1 := r.Group("/api/v1")
	// imports.RegisterRoutes(v1.Group("/imports"))
	// exports.RegisterRoutes(v1.Group("/exports"))

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s...", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
