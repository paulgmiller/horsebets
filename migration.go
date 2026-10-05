package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

type migrationData struct {
	Races  []Race
	Horses []Horse
	Bets   []Bet
}

func readMigrationData(db *gorm.DB) (migrationData, error) {
	var data migrationData
	for _, model := range []interface{}{&data.Races, &data.Horses, &data.Bets} {
		if err := db.Order("id").Find(model).Error; err != nil {
			return data, err
		}
	}
	// CockroachDB timestamps have microsecond precision.
	for i := range data.Races {
		data.Races[i].CreatedAt = data.Races[i].CreatedAt.UTC().Truncate(time.Microsecond)
	}
	for i := range data.Bets {
		data.Bets[i].CreatedAt = data.Bets[i].CreatedAt.UTC().Truncate(time.Microsecond)
	}
	return data, nil
}

func copyMigrationData(dst *gorm.DB, data migrationData) error {
	for attempt := 0; attempt < 5; attempt++ {
		err := dst.Transaction(func(tx *gorm.DB) error {
			for _, model := range []interface{}{&Race{}, &Horse{}, &Bet{}} {
				var count int64
				if err := tx.Model(model).Count(&count).Error; err != nil {
					return err
				}
				if count != 0 {
					return fmt.Errorf("destination tables must be empty; %T contains %d rows", model, count)
				}
			}
			// Maps bypass GORM's timestamp and association callbacks, preserving source values.
			for _, race := range data.Races {
				if err := tx.Model(&Race{}).Create(map[string]interface{}{
					"id": race.ID, "name": race.Name, "created_at": race.CreatedAt, "locked": race.Locked,
				}).Error; err != nil {
					return fmt.Errorf("copy race %d: %w", race.ID, err)
				}
			}
			for _, horse := range data.Horses {
				if err := tx.Model(&Horse{}).Create(map[string]interface{}{
					"id": horse.ID, "race_id": horse.RaceID, "name": horse.Name, "amount": horse.Amount,
				}).Error; err != nil {
					return fmt.Errorf("copy horse %d: %w", horse.ID, err)
				}
			}
			for _, bet := range data.Bets {
				if err := tx.Model(&Bet{}).Create(map[string]interface{}{
					"id": bet.ID, "horse_id": bet.HorseID, "name": bet.Name, "amount": bet.Amount, "created_at": bet.CreatedAt,
				}).Error; err != nil {
					return fmt.Errorf("copy bet %d: %w", bet.ID, err)
				}
			}
			actual, err := readMigrationData(tx)
			if err != nil {
				return fmt.Errorf("verify destination: %w", err)
			}
			if !reflect.DeepEqual(data, actual) {
				return fmt.Errorf("destination rows differ from source; rolling back")
			}
			return nil
		})
		var state interface{ SQLState() string }
		if !errors.As(err, &state) || state.SQLState() != "40001" || attempt == 4 {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	return fmt.Errorf("migration retries exhausted")
}

func runDataMigration() error {
	if os.Getenv("DATABASE_URL") == "" || os.Getenv("MSSQL_DSN") == "" {
		return fmt.Errorf("migrate-data requires DATABASE_URL and MSSQL_DSN")
	}
	src, err := gorm.Open(sqlserver.Open(os.Getenv("MSSQL_DSN")), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open SQL Server source: %w", err)
	}
	sourceSQL, err := src.DB()
	if err != nil {
		return err
	}
	defer sourceSQL.Close()
	dst, err := openDatabase()
	if err != nil {
		return err
	}
	destinationSQL, err := dst.DB()
	if err != nil {
		return err
	}
	defer destinationSQL.Close()
	if err := migrateSchema(dst); err != nil {
		return fmt.Errorf("migrate destination schema: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var data migrationData
	// Hold a consistent SQL Server read transaction until the destination copy commits.
	err = src.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		data, err = readMigrationData(tx)
		if err != nil {
			return fmt.Errorf("read SQL Server source: %w", err)
		}
		return copyMigrationData(dst.WithContext(ctx), data)
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	fmt.Printf("Data migration complete: copied and verified %d races, %d horses, %d bets\n", len(data.Races), len(data.Horses), len(data.Bets))
	return nil
}
