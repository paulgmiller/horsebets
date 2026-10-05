package main

import (
	"path/filepath"
	"testing"
)

func TestLocalDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SQLLITE_FILE", filepath.Join(t.TempDir(), "bets.db"))
	database, err := openDatabase()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	// Startup migration must also succeed against an existing schema.
	for i := 0; i < 2; i++ {
		if err := migrateSchema(database); err != nil {
			t.Fatal(err)
		}
	}
	race := Race{Name: "Derby", Horses: []Horse{{Name: "Comet"}}}
	if err := database.Create(&race).Error; err != nil {
		t.Fatal(err)
	}
	bet := Bet{HorseID: race.Horses[0].ID, Name: "Ada", Amount: 5}
	if err := database.Create(&bet).Error; err != nil {
		t.Fatal(err)
	}
	var loaded Race
	if err := database.Preload("Horses.Bets").First(&loaded, race.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(loaded.Horses) != 1 || len(loaded.Horses[0].Bets) != 1 || loaded.Horses[0].Bets[0].Name != "Ada" {
		t.Fatalf("race associations were not preserved: %+v", loaded)
	}
}

func TestInvalidDatabaseURLDoesNotFallBack(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://%invalid")
	t.Setenv("SQLLITE_FILE", filepath.Join(t.TempDir(), "bets.db"))
	if _, err := openDatabase(); err == nil {
		t.Fatal("expected invalid DATABASE_URL to fail instead of opening SQLite")
	}
}
