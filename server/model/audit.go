package model

import (
	"time"

	"gorm.io/datatypes"
)

// AuditLog records important player and system actions.
type AuditLog struct {
	ID         int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	TraceID    string         `gorm:"index:idx_audit_trace;size:36;not null" json:"trace_id"`
	CharID     *int64         `gorm:"index:idx_audit_char" json:"char_id"`
	AccountID  *int64         `json:"account_id"`
	CharName   string         `gorm:"size:32" json:"char_name"`
	Action     string         `gorm:"size:64;not null" json:"action"`
	Request    datatypes.JSON `json:"request"`
	Response   datatypes.JSON `json:"response"`
	Error      string         `gorm:"type:text" json:"error"`
	IP         string         `gorm:"size:45" json:"ip"`
	MapID      int            `json:"map_id"`
	DurationMs int            `json:"duration_ms"`
	CreatedAt  time.Time      `gorm:"index:idx_audit_created;autoCreateTime:milli" json:"created_at"`
}
