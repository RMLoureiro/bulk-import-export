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
	Status      string     `gorm:"not null;default:'draft'" json:"status"` // draft or published
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `gorm:"not null" json:"created_at"`
	
	// Relations
	Tags []TagModel `gorm:"many2many:article_tags;joinForeignKey:article_id;joinReferences:tag_id" json:"tags,omitempty"`
}

// TagModel for separate tag storage (enables filtering)
type TagModel struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"uniqueIndex;not null" json:"name"`
}

// ArticleTag is the join table for article-tag many-to-many relationship
type ArticleTag struct {
	ArticleID string `gorm:"primaryKey;type:text"`
	TagID     uint   `gorm:"primaryKey"`
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

func (TagModel) TableName() string {
	return "tags"
}

func (ArticleTag) TableName() string {
	return "article_tags"
}

// AutoMigrate creates the articles, comments, tags, and article_tags tables
func AutoMigrate() {
	db := common.GetDB()
	
	// Create tables
	db.AutoMigrate(&ArticleModel{}, &CommentModel{}, &TagModel{}, &ArticleTag{})
	
	// Add indexes for foreign keys
	db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_articles_author_id ON articles(author_id);
		CREATE INDEX IF NOT EXISTS idx_comments_article_id ON comments(article_id);
		CREATE INDEX IF NOT EXISTS idx_comments_user_id ON comments(user_id);
	`)
	
	// Enable foreign key constraints (SQLite 3.6.19+)
	db.Exec("PRAGMA foreign_keys = ON")
}
