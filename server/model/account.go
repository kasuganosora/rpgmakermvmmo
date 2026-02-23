package model

import "time"

// Account represents a player account.
type Account struct {
	ID           int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:32;not null" json:"username"`
	PasswordHash string     `gorm:"size:64;not null" json:"-"`
	Email        string     `gorm:"size:128" json:"email"`
	Status       int        `gorm:"default:1" json:"status"` // 0=banned 1=normal
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	LastLoginIP  string     `gorm:"size:45" json:"last_login_ip"`
}
