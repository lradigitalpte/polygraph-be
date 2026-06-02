package database

import (
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// InitDB initializes the database connection
func InitDB() (*gorm.DB, error) {
	var err error
	dsn := os.Getenv("DATABASE_URL")
	usingSQLite := false

	if dsn == "" {
		// Default to SQLite for development
		usingSQLite = true
		DB, err = gorm.Open(sqlite.Open("app.db"), &gorm.Config{})
	} else if strings.HasPrefix(dsn, "postgres") {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	} else {
		// Assume SQLite file path
		usingSQLite = true
		DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	}

	if err != nil {
		return nil, err
	}

	if usingSQLite {
		pragmas := []string{
			"PRAGMA journal_mode=WAL;",
			"PRAGMA synchronous=NORMAL;",
			"PRAGMA temp_store=MEMORY;",
			"PRAGMA busy_timeout=5000;",
			"PRAGMA foreign_keys=ON;",
		}
		for _, pragma := range pragmas {
			if execErr := DB.Exec(pragma).Error; execErr != nil {
				return nil, execErr
			}
		}
	} else {
		// Postgres (Neon): keep a pool of warm connections so requests don't pay the
		// TLS handshake + auth round-trip to a remote region on every query.
		if sqlDB, dbErr := DB.DB(); dbErr == nil {
			sqlDB.SetMaxOpenConns(20)
			sqlDB.SetMaxIdleConns(10)
			sqlDB.SetConnMaxIdleTime(4 * time.Minute) // recycle before Neon drops idle conns
			sqlDB.SetConnMaxLifetime(30 * time.Minute)
		}
	}

	return DB, nil
}

// GetDB returns the database instance
func GetDB() *gorm.DB {
	return DB
}
