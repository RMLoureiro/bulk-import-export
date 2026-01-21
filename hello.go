package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	
	"bulk-import-export/articles"
	"bulk-import-export/common"
	"bulk-import-export/exports"
	"bulk-import-export/imports"
	"bulk-import-export/users"
	_ "bulk-import-export/docs"
	"gorm.io/gorm"
)

// @title Bulk Import/Export API
// @version 1.0
// @description RealWorld Conduit API with bulk import/export capabilities
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@example.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /v1
// @schemes http

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

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

	// Add metrics middleware
	r.Use(common.MetricsMiddleware())

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// API v1 routes
	v1 := r.Group("/v1")
	{
		v1.POST("/imports", imports.CreateImport)
		v1.GET("/imports/:job_id", imports.GetImport)
		
		v1.GET("/exports", exports.StreamExport)
		v1.POST("/exports", exports.CreateExport)
		v1.GET("/exports/:job_id", exports.GetExport)
	}
	
	// Swagger documentation route
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Serve static export files
	r.Static("/exports", "./data/exports")

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
