package main

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strconv"
	"strings"
	"time"
)

var errNotFound = errors.New("not found")
var errRaceLocked = errors.New("betting closed for this race")

type Store interface {
	Ready() error
	ListRaces() ([]Race, error)
	GetRace(id string) (*Race, error)
	HorsesForRace(id string) ([]Horse, error)
	BettorsForRace(id string) ([]string, error)
	CreateRace(name string, horseNames []string) error
	CreateBets(raceID, bettorName string, amounts map[string]float64) error
	GetHorse(id string) (*Horse, error)
	BetsForHorse(id string) ([]Bet, error)
	LockRace(id string) error
	BetsForBettor(raceID, bettorName string) ([]BettorBet, error)
}

type BettorBet struct {
	HorseID   string
	HorseName string
	Amount    float64
}

func newPublicID() (string, error) {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])), nil
}

func mustPublicID() string {
	id, err := newPublicID()
	if err != nil {
		panic(err)
	}
	return id
}

func (r Race) RouteID() string {
	if r.PublicID != "" {
		return r.PublicID
	}
	return strconv.FormatUint(uint64(r.ID), 10)
}

func (h Horse) RouteID() string {
	if h.PublicID != "" {
		return h.PublicID
	}
	return strconv.FormatUint(uint64(h.ID), 10)
}

func (h Horse) RaceRouteID() string {
	if h.RacePublicID != "" {
		return h.RacePublicID
	}
	return strconv.FormatUint(uint64(h.RaceID), 10)
}

func (b Bet) RouteID() string {
	if b.PublicID != "" {
		return b.PublicID
	}
	return strconv.FormatUint(uint64(b.ID), 10)
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
