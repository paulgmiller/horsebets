package main

import (
	"fmt"
	"math"
)

// computeFractionalOdds converts a profit multiplier into fractional odds string
func computeFractionalOdds(profit float64) string {
	if profit <= 0 {
		return "N/A"
	}
	if profit >= 1 {
		num := int(math.Round(profit * 100))
		den := 100
		g := gcd(num, den)
		num /= g
		den /= g
		if den > 4 {
			rounded := int(math.Round(profit))
			return fmt.Sprintf("%d:1", rounded)
		}
		return fmt.Sprintf("%d:%d", num, den)
	}
	// handle profit < 1, fractional odds for favorites
	num := 100
	den := int(math.Round(100 * (1 / profit)))
	g := gcd(num, den)
	num /= g
	den /= g
	return fmt.Sprintf("%d:%d", num, den)
}

// gcd computes the greatest common divisor of two ints
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
