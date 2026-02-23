package model

import "time"

// Character represents a player's in-game character.
type Character struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AccountID int64     `gorm:"index:idx_account;not null" json:"account_id"`
	Name      string    `gorm:"uniqueIndex;size:32;not null" json:"name"`
	ClassID   int       `gorm:"not null" json:"class_id"`
	WalkName  string    `gorm:"size:64" json:"walk_name"`
	WalkIndex int       `gorm:"default:0" json:"walk_index"`
	FaceName  string    `gorm:"size:64" json:"face_name"`
	FaceIndex int       `gorm:"default:0" json:"face_index"`
	Level     int       `gorm:"default:1" json:"level"`
	Exp       int64     `gorm:"default:0" json:"exp"`
	HP        int       `gorm:"not null" json:"hp"`
	MP        int       `gorm:"not null" json:"mp"`
	MaxHP     int       `gorm:"not null" json:"max_hp"`
	MaxMP     int       `gorm:"not null" json:"max_mp"`
	Atk       int       `gorm:"default:10" json:"atk"`
	Def       int       `gorm:"default:5" json:"def"`
	Mat       int       `gorm:"default:10" json:"mat"`
	Mdf       int       `gorm:"default:5" json:"mdf"`
	Agi       int       `gorm:"default:10" json:"agi"`
	Luk       int       `gorm:"default:10" json:"luk"`
	Gold      int64     `gorm:"default:0" json:"gold"`
	MapID     int       `gorm:"default:1" json:"map_id"`
	MapX      int       `gorm:"default:5" json:"map_x"`
	MapY      int       `gorm:"default:5" json:"map_y"`
	Direction int       `gorm:"default:2" json:"direction"` // 2=down 4=left 6=right 8=up
	PartyID   *int64    `json:"party_id"`
	GuildID   *int64    `json:"guild_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
