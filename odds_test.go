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
		profit float64
		want   string
	}{
		{-1.0, "N/A"},
		{0.0, "N/A"},
		{0.8, "4:5"},
		{0.1, "1:10"},
		{2.0, "2:1"},
		{1.0, "1:1"},
		{1.25, "5:4"},
		{7.12, "7:1"},
		{0.123, "100:813"},
	} {
		got := computeFractionalOdds(tc.profit)
		if got != tc.want {
			t.Errorf("computeFractionalOdds(%.3f) = %q; want %q", tc.profit, got, tc.want)
		}
	}
}
