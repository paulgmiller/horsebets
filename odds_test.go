package main

import "testing"

func TestGCD(t *testing.T) {
	for _, tc := range []struct{ a, b, want int }{
		{0, 0, 0},
		{10, 5, 5},
		{14, 21, 7},
		{17, 13, 1},
		{100, 25, 25},
	} {
		got := gcd(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("gcd(%d, %d) = %d; want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestComputeFractionalOdds(t *testing.T) {
	for _, tc := range []struct {
		total int
		stake int
		want  string
	}{
		{0, 0, "No Bets"},
		{100, 0, "No Bets"},
		{8, 10, "4:5"},
		{10, 100, "1:10"},
		{8, 4, "2:1"},
		{50, 50, "1:1"},
		{25, 20, "5:4"},
		{7, 1, "7:1"},
		{100, 813, "100:813"},
	} {
		got := computeFractionalOdds(tc.total, tc.stake)
		if got != tc.want {
			t.Errorf("computeFractionalOdds(%d, %d) = %q; want %q", tc.total, tc.stake, got, tc.want)
		}
	}
}
