package main

import (
	"fmt"
)

// computeFractionalOdds converts a profit multiplier into fractional odds string
func computeFractionalOdds(total, stake int) string {
	if stake <= 0 {
		return "No Bets"
	}
	g := gcd(total, stake)
	total /= g
	stake /= g
	return fmt.Sprintf("%d:%d", total, stake)
}

// gcd computes the greatest common divisor of two ints
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
