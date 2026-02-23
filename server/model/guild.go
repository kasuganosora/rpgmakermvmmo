package model

import "time"

// GuildRank represents a member's rank within the guild.
type GuildRank = int

const (
	GuildRankLeader     GuildRank = 1
	GuildRankViceLeader GuildRank = 2
	GuildRankMember     GuildRank = 3
)

// Guild represents a player guild/clan.
type Guild struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:32;not null" json:"name"`
	Level     int       `gorm:"default:1" json:"level"`
	Gold      int64     `gorm:"default:0" json:"gold"`
	Notice    string    `gorm:"type:text" json:"notice"`
	LeaderID  int64     `gorm:"not null" json:"leader_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// GuildMember links a character to a guild with a rank.
type GuildMember struct {
	GuildID  int64     `gorm:"primaryKey;index:idx_guild_member" json:"guild_id"`
	CharID   int64     `gorm:"primaryKey;index:idx_char_guild" json:"char_id"`
	Rank     int       `gorm:"default:3" json:"rank"`
	JoinedAt time.Time `gorm:"autoCreateTime" json:"joined_at"`
}
