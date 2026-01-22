package users

import (
	"time"

	"bulk-import-export/common"
)

// UserModel represents the spec-compliant user structure
// Spec fields: id, email, name, role, active, created_at, updated_at
type UserModel struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	Email     string    `gorm:"uniqueIndex;not null" json:"email"`
	Name      string    `gorm:"not null" json:"name"`
	Role      string    `gorm:"not null" json:"role"`
	Active    bool      `gorm:"not null" json:"active"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (UserModel) TableName() string {
	return "users"
}

// AutoMigrate creates the users table
func AutoMigrate() {
	db := common.GetDB()
	db.AutoMigrate(&UserModel{})
}
