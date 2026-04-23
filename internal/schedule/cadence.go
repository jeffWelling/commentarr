// Package schedule holds the adaptive scan-cadence math from DESIGN.md
// § 5.12. Pure functions — no I/O, no external state.
package schedule

import (
	"math/rand"
	"time"
)

const (
	weekly     = 7 * 24 * time.Hour
	monthly    = 30 * 24 * time.Hour
	sixMonthly = 180 * 24 * time.Hour
)

// NextSearchInterval returns the time between successive searches of a
// single wanted title, based on the total number of titles in the user's
// library. Shorter intervals for small libraries (snappier hunting);
// longer intervals for huge libraries (don't overwhelm indexers).
//
// The returned interval is jittered ±20% (seeded by seed) so a cohort of
// titles scheduled "at the same moment" doesn't all fire together.
func NextSearchInterval(librarySize int, seed int64) time.Duration {
	var base time.Duration
	switch {
	case librarySize < 1000:
		base = weekly
	case librarySize < 10000:
		base = monthly
	default:
		base = sixMonthly
	}
	// deterministic jitter in [-20%, +20%]
	r := rand.New(rand.NewSource(seed))
	jitter := (r.Float64()*0.4 - 0.2) * float64(base)
	return base + time.Duration(jitter)
}
