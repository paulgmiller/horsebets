package main

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	docTypeRace  = "race"
	docTypeHorse = "horse"
	docTypeBet   = "bet"
)

type cosmosStore struct {
	shim *cosmosShim
}

func newCosmosStoreFromEnv() (*cosmosStore, bool, error) {
	shim, ok, err := newCosmosShimFromEnv("horsebets", "races", "/race_public_id")
	if !ok || err != nil {
		return nil, ok, err
	}
	return &cosmosStore{shim: shim}, true, nil
}

func (s *cosmosStore) Ready() error {
	return s.shim.ready()
}

func (s *cosmosStore) ListRaces() ([]Race, error) {
	rows, err := s.shim.queryMaps(
		fmt.Sprintf("SELECT CROSS PARTITION c.id,c.public_id,c.race_public_id,c.doc_type,c.name,c.created_at,c.locked FROM %s c WHERE c.doc_type=@1", s.shim.container),
		docTypeRace,
	)
	if err != nil {
		return nil, err
	}
	races := make([]Race, 0, len(rows))
	for _, row := range rows {
		race, err := raceFromCosmos(row)
		if err != nil {
			return nil, err
		}
		races = append(races, race)
	}
	sort.Slice(races, func(i, j int) bool {
		return races[i].CreatedAt.After(races[j].CreatedAt)
	})
	return races, nil
}

func (s *cosmosStore) GetRace(id string) (*Race, error) {
	rows, err := s.shim.queryMaps(
		fmt.Sprintf("SELECT c.id,c.public_id,c.race_public_id,c.doc_type,c.name,c.created_at,c.locked FROM %s c WHERE c.race_public_id=@1 AND c.doc_type=@2", s.shim.container),
		id,
		docTypeRace,
	)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errNotFound
	}
	race, err := raceFromCosmos(rows[0])
	if err != nil {
		return nil, err
	}
	return &race, nil
}

func (s *cosmosStore) HorsesForRace(id string) ([]Horse, error) {
	race, err := s.GetRace(id)
	if err != nil {
		return nil, err
	}
	rows, err := s.shim.queryMaps(
		fmt.Sprintf("SELECT c.id,c.public_id,c.race_public_id,c.doc_type,c.name,c.amount FROM %s c WHERE c.race_public_id=@1 AND c.doc_type=@2", s.shim.container),
		race.RouteID(),
		docTypeHorse,
	)
	if err != nil {
		return nil, err
	}
	horses := make([]Horse, 0, len(rows))
	for _, row := range rows {
		horses = append(horses, horseFromCosmos(row))
	}
	sort.Slice(horses, func(i, j int) bool {
		return horses[i].Name < horses[j].Name
	})
	return horses, nil
}

func (s *cosmosStore) BettorsForRace(id string) ([]string, error) {
	race, err := s.GetRace(id)
	if err != nil {
		return nil, err
	}
	rows, err := s.shim.queryMaps(
		fmt.Sprintf("SELECT c.name FROM %s c WHERE c.race_public_id=@1 AND c.doc_type=@2", s.shim.container),
		race.RouteID(),
		docTypeBet,
	)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	bettors := []string{}
	for _, row := range rows {
		name := stringValue(row["name"])
		if name != "" && !seen[name] {
			seen[name] = true
			bettors = append(bettors, name)
		}
	}
	sort.Strings(bettors)
	return bettors, nil
}

func (s *cosmosStore) CreateRace(name string, horseNames []string) error {
	raceID, err := newPublicID()
	if err != nil {
		return err
	}
	createdAt := nowUTC()
	if err := s.shim.exec(
		fmt.Sprintf("INSERT INTO %s (id,public_id,race_public_id,doc_type,name,created_at,locked) VALUES (@1,@2,@3,@4,@5,@6,@7) WITH PK=/race_public_id", s.shim.container),
		cosmosRaceID(raceID),
		raceID,
		raceID,
		docTypeRace,
		name,
		createdAt.Format(time.RFC3339Nano),
		false,
	); err != nil {
		return err
	}
	for _, horseName := range horseNames {
		horseID, err := newPublicID()
		if err != nil {
			return err
		}
		if err := s.shim.exec(
			fmt.Sprintf("INSERT INTO %s (id,public_id,race_public_id,doc_type,name,amount) VALUES (@1,@2,@3,@4,@5,@6) WITH PK=/race_public_id", s.shim.container),
			cosmosHorseID(raceID, horseID),
			horseID,
			raceID,
			docTypeHorse,
			horseName,
			0,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *cosmosStore) CreateBets(raceID, bettorName string, amounts map[string]float64) error {
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
		if err := s.shim.exec(
			fmt.Sprintf("INSERT INTO %s (id,public_id,race_public_id,horse_public_id,doc_type,name,amount,created_at) VALUES (@1,@2,@3,@4,@5,@6,@7,@8) WITH PK=/race_public_id", s.shim.container),
			cosmosBetID(race.RouteID(), betID),
			betID,
			race.RouteID(),
			horse.RouteID(),
			docTypeBet,
			bettorName,
			amount,
			nowUTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
		horse.Amount += amount
		if err := s.updateHorseAmount(race.RouteID(), horse.RouteID(), horse.Amount); err != nil {
			return err
		}
	}
	return nil
}

func (s *cosmosStore) GetHorse(id string) (*Horse, error) {
	rows, err := s.shim.queryMaps(
		fmt.Sprintf("SELECT CROSS PARTITION c.id,c.public_id,c.race_public_id,c.doc_type,c.name,c.amount FROM %s c WHERE c.public_id=@1 AND c.doc_type=@2", s.shim.container),
		id,
		docTypeHorse,
	)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errNotFound
	}
	horse := horseFromCosmos(rows[0])
	return &horse, nil
}

func (s *cosmosStore) BetsForHorse(id string) ([]Bet, error) {
	horse, err := s.GetHorse(id)
	if err != nil {
		return nil, err
	}
	rows, err := s.shim.queryMaps(
		fmt.Sprintf("SELECT c.id,c.public_id,c.race_public_id,c.horse_public_id,c.doc_type,c.name,c.amount,c.created_at FROM %s c WHERE c.race_public_id=@1 AND c.horse_public_id=@2 AND c.doc_type=@3", s.shim.container),
		horse.RaceRouteID(),
		horse.RouteID(),
		docTypeBet,
	)
	if err != nil {
		return nil, err
	}
	bets := make([]Bet, 0, len(rows))
	for _, row := range rows {
		bet, err := betFromCosmos(row)
		if err != nil {
			return nil, err
		}
		bets = append(bets, bet)
	}
	return bets, nil
}

func (s *cosmosStore) LockRace(id string) error {
	race, err := s.GetRace(id)
	if err != nil {
		return err
	}
	return s.shim.exec(
		fmt.Sprintf("UPDATE %s SET locked=@3 WHERE id=@1 AND race_public_id=@2", s.shim.container),
		cosmosRaceID(race.RouteID()),
		race.RouteID(),
		true,
	)
}

func (s *cosmosStore) BetsForBettor(raceID, bettorName string) ([]BettorBet, error) {
	horses, err := s.HorsesForRace(raceID)
	if err != nil {
		return nil, err
	}
	horseByID := map[string]Horse{}
	for _, horse := range horses {
		horseByID[horse.RouteID()] = horse
	}
	race, err := s.GetRace(raceID)
	if err != nil {
		return nil, err
	}
	rows, err := s.shim.queryMaps(
		fmt.Sprintf("SELECT c.horse_public_id,c.name,c.amount FROM %s c WHERE c.race_public_id=@1 AND c.doc_type=@2 AND c.name=@3", s.shim.container),
		race.RouteID(),
		docTypeBet,
		bettorName,
	)
	if err != nil {
		return nil, err
	}
	bettorBets := make([]BettorBet, 0, len(rows))
	for _, row := range rows {
		horseID := stringValue(row["horse_public_id"])
		horse := horseByID[horseID]
		bettorBets = append(bettorBets, BettorBet{
			HorseID:   horse.RouteID(),
			HorseName: horse.Name,
			Amount:    floatValue(row["amount"]),
		})
	}
	return bettorBets, nil
}

func (s *cosmosStore) updateHorseAmount(raceID, horseID string, amount float64) error {
	return s.shim.exec(
		fmt.Sprintf("UPDATE %s SET amount=@3 WHERE id=@1 AND race_public_id=@2", s.shim.container),
		cosmosHorseID(raceID, horseID),
		raceID,
		amount,
	)
}

func raceFromCosmos(row map[string]any) (Race, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, stringValue(row["created_at"]))
	if err != nil {
		createdAt = time.Time{}
	}
	return Race{
		PublicID:  stringValue(row["public_id"]),
		Name:      stringValue(row["name"]),
		CreatedAt: createdAt,
		Locked:    boolValue(row["locked"]),
	}, nil
}

func horseFromCosmos(row map[string]any) Horse {
	return Horse{
		PublicID:     stringValue(row["public_id"]),
		RacePublicID: stringValue(row["race_public_id"]),
		Name:         stringValue(row["name"]),
		Amount:       floatValue(row["amount"]),
	}
}

func betFromCosmos(row map[string]any) (Bet, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, stringValue(row["created_at"]))
	if err != nil {
		createdAt = time.Time{}
	}
	return Bet{
		PublicID:      stringValue(row["public_id"]),
		RacePublicID:  stringValue(row["race_public_id"]),
		HorsePublicID: stringValue(row["horse_public_id"]),
		Name:          stringValue(row["name"]),
		Amount:        floatValue(row["amount"]),
		CreatedAt:     createdAt,
	}, nil
}

func cosmosRaceID(publicID string) string {
	return "race:" + publicID
}

func cosmosHorseID(raceID, horseID string) string {
	return "horse:" + raceID + ":" + horseID
}

func cosmosBetID(raceID, betID string) string {
	return "bet:" + raceID + ":" + url.PathEscape(betID)
}

func stringValue(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return fmt.Sprint(value)
	}
}

func floatValue(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case []byte:
		var parsed float64
		_, _ = fmt.Sscan(string(value), &parsed)
		return parsed
	case string:
		var parsed float64
		_, _ = fmt.Sscan(value, &parsed)
		return parsed
	default:
		return 0
	}
}

func boolValue(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case []byte:
		return strings.EqualFold(string(value), "true")
	case string:
		return strings.EqualFold(value, "true")
	default:
		return false
	}
}
