package schedule

import (
	"testing"
	"time"
)

func TestNextSearchInterval_AdaptiveBySize(t *testing.T) {
	cases := []struct {
		name        string
		librarySz   int
		wantBetween [2]time.Duration
	}{
		{"small library weekly", 500, [2]time.Duration{6 * 24 * time.Hour, 8 * 24 * time.Hour + time.Hour}},
		{"small boundary", 999, [2]time.Duration{6 * 24 * time.Hour, 8 * 24 * time.Hour + time.Hour}},
		{"medium library monthly", 5000, [2]time.Duration{24 * 24 * time.Hour, 36 * 24 * time.Hour + time.Hour}},
		{"medium boundary", 9999, [2]time.Duration{24 * 24 * time.Hour, 36 * 24 * time.Hour + time.Hour}},
		{"large library 6-monthly", 20000, [2]time.Duration{144 * 24 * time.Hour, 216 * 24 * time.Hour + time.Hour}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NextSearchInterval(c.librarySz, 42)
			if got < c.wantBetween[0] || got > c.wantBetween[1] {
				t.Fatalf("librarySz=%d: want %v..%v got %v", c.librarySz, c.wantBetween[0], c.wantBetween[1], got)
			}
		})
	}
}

func TestNextSearchInterval_JitterStableForSameSeed(t *testing.T) {
	a := NextSearchInterval(500, 1)
	b := NextSearchInterval(500, 1)
	if a != b {
		t.Fatalf("same seed should yield same interval, got %v vs %v", a, b)
	}
}

func TestNextSearchInterval_DifferentSeedsDiffer(t *testing.T) {
	a := NextSearchInterval(500, 1)
	b := NextSearchInterval(500, 2)
	if a == b {
		t.Fatalf("different seeds should typically yield different jittered intervals")
	}
}

func TestNextSearchInterval_StaysWithin20PctOfBase(t *testing.T) {
	// Sample 50 seeds at each library size; all should fall within ±20% of base.
	cases := []struct {
		size int
		base time.Duration
	}{
		{500, 7 * 24 * time.Hour},
		{5000, 30 * 24 * time.Hour},
		{20000, 180 * 24 * time.Hour},
	}
	for _, c := range cases {
		lo := time.Duration(float64(c.base) * 0.8)
		hi := time.Duration(float64(c.base) * 1.2)
		for seed := int64(0); seed < 50; seed++ {
			got := NextSearchInterval(c.size, seed)
			if got < lo || got > hi {
				t.Fatalf("size=%d seed=%d: %v outside [%v, %v]", c.size, seed, got, lo, hi)
			}
		}
	}
}
