package main

import (
	"math"
)

// Common bookmaker-friendly fractional odds as float equivalents
var traditionalOdds = []struct {
	Frac string
	Dec  float64
}{
	{"1:10", 0.1},
	{"1:5", 0.2},
	{"2:11", 0.18},
	{"1:4", 0.25},
	{"2:7", 0.29},
	{"1:3", 0.33},
	{"4:11", 0.36},
	{"2:5", 0.4},
	{"1:2", 0.5},
	{"4:7", 0.57},
	{"8:11", 0.73},
	{"4:5", 0.8},
	{"10:11", 0.91},
	{"1:1", 1.0},
	{"6:5", 1.2},
	{"5:4", 1.25},
	{"11:8", 1.38},
	{"7:5", 1.4},
	{"6:4", 1.5},
	{"13:8", 1.63},
	{"15:8", 1.88},
	{"2:1", 2.0},
	{"9:4", 2.25},
	{"5:2", 2.5},
	{"11:4", 2.75},
	{"3:1", 3.0},
	{"7:2", 3.5},
	{"4:1", 4.0},
	{"9:2", 4.5},
	{"5:1", 5.0},
	{"6:1", 6.0},
	{"7:1", 7.0},
	{"8:1", 8.0},
	{"9:1", 9.0},
	{"10:1", 10.0},
	{"12:1", 12.0},
	{"14:1", 14.0},
	{"16:1", 16.0},
	{"20:1", 20.0},
	{"25:1", 25.0},
	{"33:1", 33.0},
	{"40:1", 40.0},
	{"50:1", 50.0},
	{"66:1", 66.0},
	{"100:1", 100.0},
}

func computeFractionalOdds(totalPot, stake float64) string {
	if stake <= 0 {
		return "No Bets"
	}

	decimalOdds := totalPot / stake
	bestMatch := traditionalOdds[0]
	minDiff := math.Abs(decimalOdds - bestMatch.Dec)

	for _, o := range traditionalOdds {
		diff := math.Abs(decimalOdds - o.Dec)
		if diff < minDiff {
			bestMatch = o
			minDiff = diff
		}
	}

	return bestMatch.Frac
}
