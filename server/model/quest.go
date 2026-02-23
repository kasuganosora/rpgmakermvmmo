package model

import (
	"time"

	"gorm.io/datatypes"
)

// QuestStatus represents the completion state of a quest.
type QuestStatus = int

const (
	QuestStatusInProgress QuestStatus = 0
	QuestStatusCompleted  QuestStatus = 1
	QuestStatusFailed     QuestStatus = 2
)

// QuestProgress tracks a character's progress on a quest.
type QuestProgress struct {
	ID          int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	CharID      int64          `gorm:"index:idx_char_quest;not null" json:"char_id"`
	QuestID     int            `gorm:"not null" json:"quest_id"`
	Status      int            `gorm:"default:0" json:"status"`
	Progress    datatypes.JSON `json:"progress"` // {"kill_count": 3, ...}
	AcceptedAt  time.Time      `gorm:"autoCreateTime" json:"accepted_at"`
	CompletedAt *time.Time     `json:"completed_at"`
}
