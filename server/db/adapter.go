package db

import (
	"fmt"

	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	"github.com/kasuganosora/rpgmakermvmmo/server/db/embedded"
	dbmysql "github.com/kasuganosora/rpgmakermvmmo/server/db/mysql"
	dbsqlite "github.com/kasuganosora/rpgmakermvmmo/server/db/sqlite"
	"gorm.io/gorm"
)

const (
	ModeEmbeddedXML    = "embedded_xml"
	ModeEmbeddedMemory = "embedded_memory"
	ModeSQLite         = "sqlite"
	ModeMySQL          = "mysql"
)

// Open returns a *gorm.DB for the configured database mode.
func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	switch cfg.Mode {
	case ModeEmbeddedXML:
		return embedded.Open(cfg.EmbeddedPath, embedded.EngineXML)
	case ModeEmbeddedMemory:
		return embedded.Open("", embedded.EngineMemory)
	case ModeSQLite:
		return dbsqlite.Open(cfg.SQLitePath)
	case ModeMySQL:
		return dbmysql.Open(cfg.MySQLDSN, cfg.MySQLMaxOpen, cfg.MySQLMaxIdle, cfg.MySQLMaxLife)
	default:
		return nil, fmt.Errorf("db: unknown mode %q", cfg.Mode)
	}
}
