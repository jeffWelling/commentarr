package indexer

import (
	"fmt"
	"strings"
)

// BuildQueries returns commentary-biased query strings for a title. We
// emit one plain query (title + year) plus four commentary-hint
// variants. The caller uses each variant independently — most indexers
// don't support OR in a single query, so we fan out.
//
// TV titles include an SxxEyy tag when season/episode are both > 0.
func BuildQueries(title string, year, season, episode int) []string {
	base := strings.TrimSpace(title)
	if year > 0 {
		base = fmt.Sprintf("%s %d", base, year)
	}
	if season > 0 && episode > 0 {
		base = fmt.Sprintf("%s S%02dE%02d", base, season, episode)
	}

	hints := []string{
		"",
		"commentary",
		"criterion",
		"special edition",
		"collector",
	}

	seen := map[string]bool{}
	out := make([]string, 0, len(hints))
	for _, h := range hints {
		q := base
		if h != "" {
			q = base + " " + h
		}
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	return out
}
