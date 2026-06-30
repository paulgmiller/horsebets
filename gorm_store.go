package main

import (
	"errors"
	"sort"
	"strconv"

	"gorm.io/gorm"
)

type gormStore struct {
	db *gorm.DB
}

func newGormStore(db *gorm.DB) *gormStore {
	return &gormStore{db: db}
}

func (s *gormStore) Ready() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

func (s *gormStore) ListRaces() ([]Race, error) {
	var races []Race
	err := s.db.Order("created_at desc").Find(&races).Error
	return races, err
}

func (s *gormStore) GetRace(id string) (*Race, error) {
	var race Race
	result := s.db.Model(&Race{})
	if numericID, err := strconv.ParseUint(id, 10, 64); err == nil {
		result = result.First(&race, uint(numericID))
	} else {
		result = result.Where("public_id = ?", id).First(&race)
	}
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, errNotFound
		}
		return nil, result.Error
	}
	return &race, nil
}

func (s *gormStore) HorsesForRace(id string) ([]Horse, error) {
	race, err := s.GetRace(id)
	if err != nil {
		return nil, err
	}
	var horses []Horse
	query := s.db.Where("race_id = ?", race.ID)
	if race.PublicID != "" {
		query = s.db.Where("race_id = ? OR race_public_id = ?", race.ID, race.PublicID)
	}
	if err := query.Find(&horses).Error; err != nil {
		return nil, err
	}
	return horses, nil
}

func (s *gormStore) BettorsForRace(id string) ([]string, error) {
	horses, err := s.HorsesForRace(id)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var bettors []string
	for _, horse := range horses {
		var bets []Bet
		query := s.db.Where("horse_id = ?", horse.ID)
		if horse.PublicID != "" {
			query = s.db.Where("horse_id = ? OR horse_public_id = ?", horse.ID, horse.PublicID)
		}
		if err := query.Find(&bets).Error; err != nil {
			return nil, err
		}
		for _, bet := range bets {
			if !seen[bet.Name] {
				seen[bet.Name] = true
				bettors = append(bettors, bet.Name)
			}
		}
	}
	sort.Strings(bettors)
	return bettors, nil
}

func (s *gormStore) CreateRace(name string, horseNames []string) error {
	raceID, err := newPublicID()
	if err != nil {
		return err
	}
	race := Race{Name: name, PublicID: raceID, CreatedAt: nowUTC()}
	for _, horseName := range horseNames {
		horseID, err := newPublicID()
		if err != nil {
			return err
		}
		race.Horses = append(race.Horses, Horse{Name: horseName, PublicID: horseID, RacePublicID: raceID})
	}
	return s.db.Create(&race).Error
}

func (s *gormStore) CreateBets(raceID, bettorName string, amounts map[string]float64) error {
	race, err := s.GetRace(raceID)
	if err != nil {
		return err
	}
	if race.Locked {
		return errRaceLocked
	}
	horses, err := s.HorsesForRace(raceID)
	if err != nil {
		return err
	}
	for _, horse := range horses {
		amount, ok := amounts[horse.RouteID()]
		if !ok || amount <= 0 {
			continue
		}
		betID, err := newPublicID()
		if err != nil {
			return err
		}
		bet := Bet{
			PublicID:      betID,
			HorseID:       horse.ID,
			HorsePublicID: horse.PublicID,
			RacePublicID:  race.RouteID(),
			Name:          bettorName,
			Amount:        amount,
			CreatedAt:     nowUTC(),
		}
		if err := s.db.Create(&bet).Error; err != nil {
			return err
		}
		if err := s.db.Model(&Horse{}).Where("id = ?", horse.ID).
			UpdateColumn("amount", gorm.Expr("amount + ?", amount)).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *gormStore) GetHorse(id string) (*Horse, error) {
	var horse Horse
	result := s.db.Model(&Horse{})
	if numericID, err := strconv.ParseUint(id, 10, 64); err == nil {
		result = result.First(&horse, uint(numericID))
	} else {
		result = result.Where("public_id = ?", id).First(&horse)
	}
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, errNotFound
		}
		return nil, result.Error
	}
	return &horse, nil
}

func (s *gormStore) BetsForHorse(id string) ([]Bet, error) {
	horse, err := s.GetHorse(id)
	if err != nil {
		return nil, err
	}
	var bets []Bet
	query := s.db.Where("horse_id = ?", horse.ID)
	if horse.PublicID != "" {
		query = s.db.Where("horse_id = ? OR horse_public_id = ?", horse.ID, horse.PublicID)
	}
	if err := query.Find(&bets).Error; err != nil {
		return nil, err
	}
	return bets, nil
}

func (s *gormStore) LockRace(id string) error {
	race, err := s.GetRace(id)
	if err != nil {
		return err
	}
	return s.db.Model(&Race{}).Where("id = ?", race.ID).Update("locked", true).Error
}

func (s *gormStore) BetsForBettor(raceID, bettorName string) ([]BettorBet, error) {
	horses, err := s.HorsesForRace(raceID)
	if err != nil {
		return nil, err
	}
	var bettorBets []BettorBet
	for _, horse := range horses {
		var bets []Bet
		query := s.db.Where("name = ?", bettorName).Where("horse_id = ?", horse.ID)
		if horse.PublicID != "" {
			query = s.db.Where("name = ?", bettorName).Where("horse_id = ? OR horse_public_id = ?", horse.ID, horse.PublicID)
		}
		if err := query.Find(&bets).Error; err != nil {
			return nil, err
		}
		for _, bet := range bets {
			bettorBets = append(bettorBets, BettorBet{
				HorseID:   horse.RouteID(),
				HorseName: horse.Name,
				Amount:    bet.Amount,
			})
		}
	}
	return bettorBets, nil
}
