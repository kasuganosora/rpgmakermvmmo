package testutil

import (
	"testing"

	dbadapter "github.com/kasuganosora/rpgmakermvmmo/server/db"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// SetupTestDB creates an in-memory embedded DB and runs AutoMigrate.
// It requires no external services and is safe to use in parallel tests.
func SetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := dbadapter.Open(config.DatabaseConfig{
		Mode: dbadapter.ModeEmbeddedMemory,
	})
	require.NoError(t, err, "SetupTestDB: Open")
	require.NoError(t, model.AutoMigrate(db), "SetupTestDB: AutoMigrate")
	return db
}

// SetupTestCache creates LocalCache and LocalPubSub (no Redis required).
func SetupTestCache(t *testing.T) (cache.Cache, cache.PubSub) {
	t.Helper()
	cfg := cache.CacheConfig{} // empty RedisAddr → LocalCache
	c, err := cache.NewCache(cfg)
	require.NoError(t, err, "SetupTestCache: NewCache")
	ps, err := cache.NewPubSub(cfg)
	require.NoError(t, err, "SetupTestCache: NewPubSub")
	return c, ps
}
