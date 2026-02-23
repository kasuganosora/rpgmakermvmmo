package model

import "time"

// ItemKind distinguishes item types from RMMV data files.
type ItemKind = int

const (
	ItemKindItem   ItemKind = 1
	ItemKindWeapon ItemKind = 2
	ItemKindArmor  ItemKind = 3
)

// Inventory represents a single item stack in a character's bag.
type Inventory struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CharID    int64     `gorm:"index:idx_char_inventory;not null" json:"char_id"`
	ItemID    int       `gorm:"not null" json:"item_id"`
	Kind      int       `gorm:"not null" json:"kind"`
	Qty       int       `gorm:"default:1" json:"qty"`
	Equipped  bool      `gorm:"default:false" json:"equipped"`
	SlotIndex int       `gorm:"default:-1" json:"slot_index"` // -1 = not equipped
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
