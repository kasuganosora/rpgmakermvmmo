package sqlite

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open creates a GORM *DB backed by SQLite (modernc.org/sqlite, no CGO).
// Enables WAL mode for better concurrent read/write performance.
func Open(path string) (*gorm.DB, error) {
	// Open with _pragma to enable WAL mode and set busy timeout
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	// Also set pragmas via Exec for older drivers
	sqlDB, err := db.DB()
	if err != nil {
		return db, nil // ignore error, connection pool not available
	}
	// Set busy timeout to 5 seconds
	_, _ = sqlDB.Exec("PRAGMA busy_timeout = 5000")
	// Enable WAL mode for better concurrency
	_, _ = sqlDB.Exec("PRAGMA journal_mode = WAL")
	// Set synchronous mode to NORMAL for better performance
	_, _ = sqlDB.Exec("PRAGMA synchronous = NORMAL")

	return db, nil
}
