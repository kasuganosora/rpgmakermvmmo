package model

import (
	"time"

	"gorm.io/datatypes"
)

// Mail represents an in-game mailbox message.
type Mail struct {
	ID         int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	ToCharID   int64          `gorm:"index:idx_mail_to;not null" json:"to_char_id"`
	FromName   string         `gorm:"size:32;default:'系统'" json:"from_name"`
	Subject    string         `gorm:"size:64" json:"subject"`
	Body       string         `gorm:"type:text" json:"body"`
	Attachment datatypes.JSON `json:"attachment"` // [{type,item_id,qty},{gold:100}]
	Claimed    int            `gorm:"default:0" json:"claimed"`
	CreatedAt  time.Time      `gorm:"autoCreateTime" json:"created_at"`
	ExpireAt   *time.Time     `json:"expire_at"`
}
