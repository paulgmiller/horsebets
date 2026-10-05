package main

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openDatabase() (*gorm.DB, error) {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		// Match aggrovites' simple-protocol workaround for CockroachDB metadata queries.
		db, err := gorm.Open(postgres.New(postgres.Config{
			DSN: url, PreferSimpleProtocol: true,
		}), &gorm.Config{})
		if err != nil {
			return nil, fmt.Errorf("open CockroachDB: %w", err)
		}
		return db, nil
	}

	filename := os.Getenv("SQLLITE_FILE")
	if filename == "" {
		filename = "bets.db"
	}
	db, err := gorm.Open(sqlite.Open(filename), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	return db, nil
}

func migrateSchema(db *gorm.DB) error {
	return db.AutoMigrate(&Race{}, &Horse{}, &Bet{})
}
