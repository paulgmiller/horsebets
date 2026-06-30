package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

var store Store

// Models
type Race struct {
	ID        uint   `gorm:"primaryKey"`
	PublicID  string `gorm:"uniqueIndex"`
	Name      string
	CreatedAt time.Time
	Locked    bool // lock betting when true
	Horses    []Horse
}

type Horse struct {
	ID           uint   `gorm:"primaryKey"`
	PublicID     string `gorm:"uniqueIndex"`
	RaceID       uint
	RacePublicID string `gorm:"index"`
	Name         string
	Amount       float64
	Bets         []Bet
}

type Bet struct {
	ID            uint   `gorm:"primaryKey"`
	PublicID      string `gorm:"uniqueIndex"`
	HorseID       uint
	HorsePublicID string `gorm:"index"`
	RacePublicID  string `gorm:"index"`
	Name          string
	Amount        float64
	CreatedAt     time.Time
}

// Data structure for odds calculation
type HorseWithOdds struct {
	Horse
	Odds           float64
	FractionalOdds string
}

// Add payout struct for template
type BetWithPayout struct {
	Name     string
	Amount   float64
	Winnings float64
}

// Templates
var (
	indexTemplate      = template.Must(template.ParseFiles("templates/index.html"))
	raceTemplate       = template.Must(template.ParseFiles("templates/race.html"))
	createRaceTemplate = template.Must(template.ParseFiles("templates/create_race.html"))
	horseTemplate      = template.Must(template.ParseFiles("templates/horse.html"))
	bettorTemplate     = template.Must(template.ParseFiles("templates/bettor.html"))
)

func main() {
	var err error
	store, err = openStore()
	if err != nil {
		log.Fatal(err)
	}

	// Routes
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/race/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stop") {
			handleStopRace(w, r)
		} else {
			handleRace(w, r)
		}
	})
	http.HandleFunc("/bet", handleBet)
	http.HandleFunc("/create", handleCreateRace)
	http.HandleFunc("/horse/", handleHorse)
	http.HandleFunc("/bettor/", handleBettor)
	http.HandleFunc("/healthz", handleHealthz)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	log.Println("Server started at http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

func openStore() (Store, error) {
	cosmos, usingCosmos, err := newCosmosStoreFromEnv()
	if err != nil {
		return nil, err
	}
	if usingCosmos {
		log.Print("using cosmos db")
		return cosmos, nil
	}

	var data gorm.Dialector
	if mssql_dsn, found := os.LookupEnv("MSSQL_DSN"); found {
		log.Printf("using mssql %s", mssql_dsn)
		data = sqlserver.Open(mssql_dsn)
	} else {
		sqllitefile := "bets.db"
		log.Printf("using sqllite db file %s", sqllitefile)
		data = sqlite.Open(sqllitefile)
	}

	db, err := gorm.Open(data, &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Auto-migrate schema
	err = db.AutoMigrate(&Race{}, &Horse{}, &Bet{})
	if err != nil {
		return nil, err
	}
	return newGormStore(db), nil
}

// Show all races
func handleHome(w http.ResponseWriter, r *http.Request) {
	races, err := store.ListRaces()
	if err != nil {
		http.Error(w, "Could not load races", http.StatusInternalServerError)
		return
	}

	indexTemplate.Execute(w, races)
}

// Show horses in a race + betting form
func handleRace(w http.ResponseWriter, r *http.Request) {
	raceIDStr := strings.TrimPrefix(r.URL.Path, "/race/")

	// Load race to get locked state
	race, err := store.GetRace(raceIDStr)
	if errors.Is(err, errNotFound) {
		http.Error(w, "Race not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Could not load race", http.StatusInternalServerError)
		return
	}

	horses, err := store.HorsesForRace(raceIDStr)
	if err != nil {
		http.Error(w, "Could not load horses", http.StatusInternalServerError)
		return
	}

	var total float64
	for _, h := range horses {
		total += h.Amount
	}

	var horsesWithOdds []HorseWithOdds
	for _, h := range horses {
		odds := 0.0
		if h.Amount > 0 {
			odds = total / h.Amount
		}
		horsesWithOdds = append(horsesWithOdds, HorseWithOdds{
			Horse:          h,
			Odds:           odds,
			FractionalOdds: computeFractionalOdds(total, h.Amount),
		})
	}

	bettors, err := store.BettorsForRace(raceIDStr)
	if err != nil {
		http.Error(w, "Could not load bettors", http.StatusInternalServerError)
		return
	}

	err = raceTemplate.Execute(w, struct {
		Name    string
		RaceID  string
		Horses  []HorseWithOdds
		Locked  bool
		Bettors []string
	}{
		Name:    race.Name,
		RaceID:  race.RouteID(),
		Horses:  horsesWithOdds,
		Locked:  race.Locked,
		Bettors: bettors,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template %s", err), http.StatusInternalServerError)
		return
	}
}

func handleBet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	bettorName := r.FormValue("name")
	raceIDStr := r.FormValue("race_id")

	if bettorName == "" {
		http.Error(w, "Missing bettor name", 400)
		return
	}

	amounts := map[string]float64{}
	for key, values := range r.Form {
		if !strings.HasPrefix(key, "amount_") || len(values) == 0 {
			continue
		}
		amountStr := values[0]
		if amountStr == "" {
			continue
		}

		amount, err := parseAmount(amountStr)
		if err != nil || amount <= 0 {
			continue
		}
		amounts[strings.TrimPrefix(key, "amount_")] = amount
	}
	if err := store.CreateBets(raceIDStr, bettorName, amounts); errors.Is(err, errRaceLocked) {
		http.Error(w, "Betting closed for this race", http.StatusForbidden)
		return
	} else if errors.Is(err, errNotFound) {
		http.Error(w, "Race not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Could not save bet", http.StatusInternalServerError)
		return
	}

	time.Sleep(300 * time.Millisecond)

	http.Redirect(w, r, "/race/"+raceIDStr, http.StatusSeeOther)
}

// Create a new race
func handleCreateRace(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		createRaceTemplate.Execute(w, nil)
		return
	}

	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		horses := r.FormValue("horses")

		if name == "" || horses == "" {
			http.Error(w, "Missing name or horses", 400)
			return
		}

		horseList := strings.Split(horses, ",")
		var horseNames []string
		for _, h := range horseList {
			h = strings.TrimSpace(h)
			if h != "" {
				horseNames = append(horseNames, h)
			}
		}

		if err := store.CreateRace(name, horseNames); err != nil {
			http.Error(w, "Could not create race", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// handleHorse shows bets for a specific horse and calculates equal share of pot
func handleHorse(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/horse/")
	horse, err := store.GetHorse(idStr)
	if errors.Is(err, errNotFound) {
		http.Error(w, "Horse not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Could not load horse", http.StatusInternalServerError)
		return
	}
	// Load bets for this horse
	bets, err := store.BetsForHorse(idStr)
	if err != nil {
		http.Error(w, "Could not load bets", http.StatusInternalServerError)
		return
	}
	// Calculate race total pot
	raceHorses, err := store.HorsesForRace(horse.RaceRouteID())
	if err != nil {
		http.Error(w, "Could not load race", http.StatusInternalServerError)
		return
	}
	var raceTotal float64
	for _, h := range raceHorses {
		raceTotal += h.Amount
	}
	// Compute payouts per bet
	var betPayouts []BetWithPayout
	for _, b := range bets {
		var ratio float64
		if horse.Amount > 0 {
			ratio = b.Amount / horse.Amount
		}
		payout := ratio * raceTotal
		betPayouts = append(betPayouts, BetWithPayout{
			Name:     b.Name,
			Amount:   b.Amount,
			Winnings: payout,
		})
	}
	// Render template with race and horse pots and payouts
	horseTemplate.Execute(w, struct {
		Horse      Horse
		Bets       []BetWithPayout
		RaceTotal  float64
		HorseTotal float64
	}{Horse: *horse, Bets: betPayouts, RaceTotal: raceTotal, HorseTotal: horse.Amount})
}

// handleStopRace locks betting on a race
func handleStopRace(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/race/"), "/stop")
	if err := store.LockRace(idStr); errors.Is(err, errNotFound) {
		http.Error(w, "Race not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Could not lock race", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/race/"+idStr, http.StatusSeeOther)
}

// handleBettor shows all bets by a specific bettor for a given race
func handleBettor(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/bettor/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	raceID := parts[0]
	name := parts[1]
	bettorBets, err := store.BetsForBettor(raceID, name)
	if errors.Is(err, errNotFound) {
		http.Error(w, "Race not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Could not load bettor", http.StatusInternalServerError)
		return
	}
	bettorTemplate.Execute(w, struct {
		RaceID string
		Name   string
		Bets   []BettorBet
	}{
		RaceID: raceID,
		Name:   name,
		Bets:   bettorBets,
	})
}

// handleHealthz checks database connectivity and liveness/readiness
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := store.Ready(); err != nil {
		http.Error(w, "Database ping failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func parseAmount(amount string) (float64, error) {
	return strconv.ParseFloat(amount, 64)
}
