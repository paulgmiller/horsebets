package main

import (
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

var db *gorm.DB

// Models
type Race struct {
	ID        uint `gorm:"primaryKey"`
	Name      string
	CreatedAt time.Time
	Locked    bool // lock betting when true
	Horses    []Horse
}

type Horse struct {
	ID     uint `gorm:"primaryKey"`
	RaceID uint
	Name   string
	Amount float64
	Bets   []Bet
}

type Bet struct {
	ID        uint `gorm:"primaryKey"`
	HorseID   uint
	Name      string
	Amount    float64
	CreatedAt time.Time
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
	var data gorm.Dialector
	if mssql_dsn, found := os.LookupEnv("MSSQL_DSN"); found {
		log.Printf("using mssql %s", mssql_dsn)
		data = sqlserver.Open(mssql_dsn)
	} else {
		sqllitefile := "bets.db"
		log.Printf("using sqllite db file %s", sqllitefile)
		data = sqlite.Open(sqllitefile)
	}

	db, err = gorm.Open(data, &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// Auto-migrate schema
	err = db.AutoMigrate(&Race{}, &Horse{}, &Bet{})
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

// Show all races
func handleHome(w http.ResponseWriter, r *http.Request) {
	var races []Race
	db.Order("created_at desc").Find(&races)

	indexTemplate.Execute(w, races)
}

// Show horses in a race + betting form
func handleRace(w http.ResponseWriter, r *http.Request) {
	raceIDStr := strings.TrimPrefix(r.URL.Path, "/race/")
	raceID, err := strconv.Atoi(raceIDStr)
	if err != nil {
		http.Error(w, "Invalid race ID", 400)
		return
	}

	// Load race to get locked state
	var race Race
	if err := db.First(&race, raceID).Error; err != nil {
		http.Error(w, "Race not found", http.StatusNotFound)
		return
	}
	locked := race.Locked

	var horses []Horse
	db.Where("race_id = ?", raceID).Find(&horses)

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

	var bettors []string
	db.Model(&Bet{}).
		Joins("join horses on bets.horse_id = horses.id").
		Where("horses.race_id = ?", raceID).
		Distinct("bets.name").
		Pluck("bets.name", &bettors)

	err = raceTemplate.Execute(w, struct {
		Name    string
		RaceID  int
		Horses  []HorseWithOdds
		Locked  bool
		Bettors []string
	}{
		Name:    race.Name,
		RaceID:  raceID,
		Horses:  horsesWithOdds,
		Locked:  locked,
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

	raceID, err := strconv.Atoi(raceIDStr)
	if err != nil {
		http.Error(w, "Invalid race ID", 400)
		return
	}

	// Check if betting is closed for this race
	var race Race
	if err := db.First(&race, raceID).Error; err != nil {
		http.Error(w, "Race not found", http.StatusNotFound)
		return
	}
	if race.Locked {
		http.Error(w, "Betting closed for this race", http.StatusForbidden)
		return
	}

	// Get all horses for this race
	var horses []Horse
	db.Where("race_id = ?", raceID).Find(&horses)

	for _, horse := range horses {
		amountStr := r.FormValue("amount_" + strconv.Itoa(int(horse.ID)))
		if amountStr == "" {
			continue
		}

		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil || amount <= 0 {
			continue
		}

		// Save individual Bet
		bet := Bet{
			HorseID: horse.ID,
			Name:    bettorName,
			Amount:  amount,
		}
		db.Create(&bet)

		// Update total amount on horse
		db.Model(&Horse{}).Where("id = ?", horse.ID).
			UpdateColumn("amount", gorm.Expr("amount + ?", amount))
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
		var horseObjs []Horse
		for _, h := range horseList {
			h = strings.TrimSpace(h)
			if h != "" {
				horseObjs = append(horseObjs, Horse{Name: h})
			}
		}

		race := Race{
			Name:   name,
			Horses: horseObjs,
		}

		db.Create(&race)

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// handleHorse shows bets for a specific horse and calculates equal share of pot
func handleHorse(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/horse/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid horse ID", http.StatusBadRequest)
		return
	}
	var horse Horse
	if err := db.First(&horse, id).Error; err != nil {
		http.Error(w, "Horse not found", http.StatusNotFound)
		return
	}
	// Load bets for this horse
	var bets []Bet
	db.Where("horse_id = ?", id).Find(&bets)
	// Calculate race total pot
	var raceHorses []Horse
	db.Where("race_id = ?", horse.RaceID).Find(&raceHorses)
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
	}{Horse: horse, Bets: betPayouts, RaceTotal: raceTotal, HorseTotal: horse.Amount})
}

// handleStopRace locks betting on a race
func handleStopRace(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/race/"), "/stop")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid race ID", http.StatusBadRequest)
		return
	}
	db.Model(&Race{}).Where("id = ?", id).Update("Locked", true)
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
	raceID, err := strconv.Atoi(parts[0])
	name := parts[1]
	if err != nil {
		http.Error(w, "Invalid race ID", http.StatusBadRequest)
		return
	}
	var horses []Horse
	db.Where("race_id = ?", raceID).Find(&horses)
	horseMap := make(map[uint]string)
	var horseIDs []uint
	for _, h := range horses {
		horseIDs = append(horseIDs, h.ID)
		horseMap[h.ID] = h.Name
	}
	var bets []Bet
	db.Where("horse_id IN ?", horseIDs).
		Where("name = ?", name).
		Find(&bets)
	type BettorBet struct {
		HorseID   uint
		HorseName string
		Amount    float64
	}
	var bettorBets []BettorBet
	for _, b := range bets {
		bettorBets = append(bettorBets, BettorBet{
			HorseID:   b.HorseID,
			HorseName: horseMap[b.HorseID],
			Amount:    b.Amount,
		})
	}
	bettorTemplate.Execute(w, struct {
		RaceID int
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
	sqlDB, err := db.DB()
	if err != nil {
		http.Error(w, "Database connection error", http.StatusInternalServerError)
		return
	}
	if err := sqlDB.Ping(); err != nil {
		http.Error(w, "Database ping failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
