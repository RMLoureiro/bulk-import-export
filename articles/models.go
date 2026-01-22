package articles

import (
	"time"

	"bulk-import-export/common"
)

// ArticleModel represents the spec-compliant article structure
// Spec fields: id, slug, title, body, author_id, tags, status, published_at
type ArticleModel struct {
	ID          string     `gorm:"primaryKey;type:text" json:"id"`
	Slug        string     `gorm:"uniqueIndex;not null" json:"slug"`
	Title       string     `gorm:"not null" json:"title"`
	Body        string     `gorm:"type:text;not null" json:"body"`
	AuthorID    string     `gorm:"not null;index" json:"author_id"`
	Tags        string     `gorm:"type:text" json:"tags"` // JSON array of tag strings
	Status      string     `gorm:"not null;default:'draft'" json:"status"` // draft or published
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `gorm:"not null" json:"created_at"`
}

// CommentModel represents the spec-compliant comment structure
// Spec fields: id, article_id, user_id, body, created_at
type CommentModel struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	ArticleID string    `gorm:"not null;index" json:"article_id"`
	UserID    string    `gorm:"not null;index" json:"user_id"`
	Body      string    `gorm:"type:text;not null" json:"body"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (ArticleModel) TableName() string {
	return "articles"
}

func (CommentModel) TableName() string {
	return "comments"
}

// AutoMigrate creates the articles and comments tables
func AutoMigrate() {
	db := common.GetDB()
	
	// Create tables
	db.AutoMigrate(&ArticleModel{}, &CommentModel{})
	
	// Add indexes for foreign keys
	db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_articles_author_id ON articles(author_id);
		CREATE INDEX IF NOT EXISTS idx_comments_article_id ON comments(article_id);
		CREATE INDEX IF NOT EXISTS idx_comments_user_id ON comments(user_id);
	`)
	
	// Enable foreign key constraints (SQLite 3.6.19+)
	db.Exec("PRAGMA foreign_keys = ON")
}
