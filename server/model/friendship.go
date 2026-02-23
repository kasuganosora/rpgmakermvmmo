package model

import "time"

// Friendship represents a friend/block relationship.
// Status: 0=pending, 1=accepted, 2=blocked.
type Friendship struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CharID    int64     `gorm:"index:idx_friendship;not null" json:"char_id"`
	FriendID  int64     `gorm:"index:idx_friendship;not null" json:"friend_id"`
	Status    int       `gorm:"default:0" json:"status"` // 0=pending,1=friend,2=blocked
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}
