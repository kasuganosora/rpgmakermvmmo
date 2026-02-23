package model_test

import (
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoMigrate_InsertAndQuery(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// Account
	acc := &model.Account{Username: "test_user", PasswordHash: "hash", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	assert.Greater(t, acc.ID, int64(0))

	var found model.Account
	require.NoError(t, db.First(&found, acc.ID).Error)
	assert.Equal(t, "test_user", found.Username)

	// Character
	char := &model.Character{
		AccountID: acc.ID,
		Name:      "Hero",
		ClassID:   1,
		HP:        100, MP: 50, MaxHP: 100, MaxMP: 50,
	}
	require.NoError(t, db.Create(char).Error)
	assert.Greater(t, char.ID, int64(0))

	// Inventory
	inv := &model.Inventory{CharID: char.ID, ItemID: 1, Kind: model.ItemKindItem, Qty: 3}
	require.NoError(t, db.Create(inv).Error)

	// Guild
	guild := &model.Guild{Name: "TestGuild", LeaderID: char.ID}
	require.NoError(t, db.Create(guild).Error)

	// GuildMember
	gm := &model.GuildMember{GuildID: guild.ID, CharID: char.ID, Rank: model.GuildRankLeader}
	require.NoError(t, db.Create(gm).Error)

	// Mail
	mail := &model.Mail{ToCharID: char.ID, FromName: "System", Subject: "Welcome"}
	require.NoError(t, db.Create(mail).Error)

	// AuditLog
	al := &model.AuditLog{
		TraceID: "trace-001", Action: "login",
		CreatedAt: time.Now(),
	}
	require.NoError(t, db.Create(al).Error)
}
