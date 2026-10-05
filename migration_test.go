package main

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func migrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "bets.db")+"?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := migrateSchema(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCopyMigrationData(t *testing.T) {
	src, dst := migrationTestDB(t), migrationTestDB(t)
	stamp := time.Date(2023, 4, 5, 6, 7, 8, 123456000, time.UTC)
	race := Race{ID: 42, Name: "Derby", CreatedAt: stamp, Locked: true}
	horse := Horse{ID: 91, RaceID: 42, Name: "Comet", Amount: 5}
	bet := Bet{ID: 100, HorseID: 91, Name: "Ada", Amount: 5, CreatedAt: stamp}
	for _, model := range []interface{}{&race, &horse, &bet} {
		if err := src.Create(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	data, err := readMigrationData(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := copyMigrationData(dst, data); err != nil {
		t.Fatal(err)
	}
	var got Race
	if err := dst.Preload("Horses.Bets").First(&got, 42).Error; err != nil {
		t.Fatal(err)
	}
	if !got.Locked || !got.CreatedAt.Equal(stamp) || len(got.Horses) != 1 || got.Horses[0].Amount != 5 || len(got.Horses[0].Bets) != 1 || got.Horses[0].Bets[0].ID != 100 {
		t.Fatalf("migration did not preserve race and bets: %+v", got)
	}
	if err := copyMigrationData(dst, data); err == nil {
		t.Fatal("expected populated destination to be rejected")
	}
}

func TestCopyMigrationDataRollsBack(t *testing.T) {
	dst := migrationTestDB(t)
	data := migrationData{
		Races:  []Race{{ID: 42, Name: "Derby"}},
		Horses: []Horse{{ID: 91, RaceID: 999, Name: "Orphan"}},
	}
	if err := copyMigrationData(dst, data); err == nil {
		t.Fatal("expected orphaned horse to fail")
	}
	var count int64
	if err := dst.Model(&Race{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("race inserted before failed horse was not rolled back")
	}
}
