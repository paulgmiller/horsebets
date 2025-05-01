package main

import "testing"

func TestComputeFractionalOdds(t *testing.T) {
	for _, tc := range []struct {
		total float64
		stake float64
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
		{100, 813, "1:10"},
	} {
		got := computeFractionalOdds(tc.total, tc.stake)
		if got != tc.want {
			t.Errorf("computeFractionalOdds(%f, %f) = %q; want %q", tc.total, tc.stake, got, tc.want)
		}
	}
}
