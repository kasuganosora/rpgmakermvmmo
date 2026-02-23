package model

import "time"

// CharSkill records which skills a character has learned.
type CharSkill struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CharID    int64     `gorm:"index:idx_char_skill;not null" json:"char_id"`
	SkillID   int       `gorm:"not null" json:"skill_id"`
	Level     int       `gorm:"default:1" json:"level"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}
