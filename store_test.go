package main

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestStore(t *testing.T) *gormStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Race{}, &Horse{}, &Bet{}); err != nil {
		t.Fatal(err)
	}
	return newGormStore(db)
}

func TestGormStoreRaceHorseAndBetFlow(t *testing.T) {
	store := newTestStore(t)
	if err := store.CreateRace("Derby", []string{"Red", "Blue"}); err != nil {
		t.Fatal(err)
	}
	races, err := store.ListRaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(races) != 1 {
		t.Fatalf("len(races) = %d, want 1", len(races))
	}
	if races[0].RouteID() == "0" || races[0].PublicID == "" {
		t.Fatalf("race route id = %q, public id = %q", races[0].RouteID(), races[0].PublicID)
	}

	horses, err := store.HorsesForRace(races[0].RouteID())
	if err != nil {
		t.Fatal(err)
	}
	if len(horses) != 2 {
		t.Fatalf("len(horses) = %d, want 2", len(horses))
	}
	if horses[0].RouteID() == "0" || horses[0].RaceRouteID() != races[0].RouteID() {
		t.Fatalf("horse route = %q race route = %q", horses[0].RouteID(), horses[0].RaceRouteID())
	}

	if err := store.CreateBets(races[0].RouteID(), "Ada", map[string]float64{horses[0].RouteID(): 10}); err != nil {
		t.Fatal(err)
	}
	updatedHorse, err := store.GetHorse(horses[0].RouteID())
	if err != nil {
		t.Fatal(err)
	}
	if updatedHorse.Amount != 10 {
		t.Fatalf("horse amount = %v, want 10", updatedHorse.Amount)
	}
	bets, err := store.BetsForHorse(horses[0].RouteID())
	if err != nil {
		t.Fatal(err)
	}
	if len(bets) != 1 || bets[0].Name != "Ada" || bets[0].RacePublicID != races[0].RouteID() {
		t.Fatalf("bets = %#v", bets)
	}
	bettors, err := store.BettorsForRace(races[0].RouteID())
	if err != nil {
		t.Fatal(err)
	}
	if len(bettors) != 1 || bettors[0] != "Ada" {
		t.Fatalf("bettors = %#v", bettors)
	}
	bettorBets, err := store.BetsForBettor(races[0].RouteID(), "Ada")
	if err != nil {
		t.Fatal(err)
	}
	if len(bettorBets) != 1 || bettorBets[0].HorseID != horses[0].RouteID() {
		t.Fatalf("bettorBets = %#v", bettorBets)
	}
}

func TestGormStoreLegacyNumericLookup(t *testing.T) {
	store := newTestStore(t)
	race := Race{Name: "Legacy"}
	if err := store.db.Create(&race).Error; err != nil {
		t.Fatal(err)
	}
	horse := Horse{RaceID: race.ID, Name: "Old"}
	if err := store.db.Create(&horse).Error; err != nil {
		t.Fatal(err)
	}

	gotRace, err := store.GetRace(race.RouteID())
	if err != nil {
		t.Fatal(err)
	}
	if gotRace.ID != race.ID {
		t.Fatalf("race ID = %d, want %d", gotRace.ID, race.ID)
	}
	gotHorse, err := store.GetHorse(horse.RouteID())
	if err != nil {
		t.Fatal(err)
	}
	if gotHorse.ID != horse.ID {
		t.Fatalf("horse ID = %d, want %d", gotHorse.ID, horse.ID)
	}
}

func TestGormStoreLockedRaceRejectsBets(t *testing.T) {
	store := newTestStore(t)
	if err := store.CreateRace("Locked", []string{"Red"}); err != nil {
		t.Fatal(err)
	}
	races, err := store.ListRaces()
	if err != nil {
		t.Fatal(err)
	}
	horses, err := store.HorsesForRace(races[0].RouteID())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.LockRace(races[0].RouteID()); err != nil {
		t.Fatal(err)
	}
	err = store.CreateBets(races[0].RouteID(), "Ada", map[string]float64{horses[0].RouteID(): 10})
	if !errors.Is(err, errRaceLocked) {
		t.Fatalf("err = %v, want errRaceLocked", err)
	}
}
