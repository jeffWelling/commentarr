// Package indexer holds the Indexer port and its v1 Prowlarr adapter.
//
// The Indexer abstraction is the one place Commentarr reaches outside the
// box for candidate releases. Every call goes through a rate-limiter +
// circuit-breaker (see ratelimit.go) so we play nice with Prowlarr and
// the trackers behind it.
package indexer

import (
	"strings"
	"time"
)

// Query is everything a search needs. For v1 we always search per-title.
// TMDB/IMDB IDs, when present, let indexers filter more accurately than
// string matching alone.
type Query struct {
	Title      string
	Year       int
	IMDBID     string
	TMDBID     string
	Categories []int // Prowlarr category ids: 2000 = Movies, 5000 = TV
	// Limit caps the number of releases returned per query. Indexers that
	// don't honour limits are expected to return more and let the caller
	// trim.
	Limit int
}

// Release is one candidate from a search. Minimal set of fields needed
// by Verifier + download hand-off. Not every indexer provides every
// field; Verifier tolerates zero values.
type Release struct {
	InfoHash    string    // lower-cased canonical hex
	URL         string    // .torrent or .nzb URL (fallback identity)
	Title       string    // release title as the indexer reports it
	SizeBytes   int64
	Seeders     int
	Leechers    int
	Indexer     string    // source indexer name
	PublishedAt time.Time
	Protocol    string    // "torrent" | "usenet" — v1 filters to torrent
}

// Identity returns a stable deduplication key derived from the richest
// uniqueness signal available. Prefers infohash, falls back to URL, then
// indexer+title+size as a last resort.
func (r Release) Identity() string {
	switch {
	case r.InfoHash != "":
		return "infohash:" + strings.ToLower(r.InfoHash)
	case r.URL != "":
		return "url:" + r.URL
	default:
		return "fallback:" + r.Indexer + ":" + r.Title
	}
}
