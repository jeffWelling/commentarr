package indexer

import (
	"strings"
	"testing"
)

func TestBuildQueries_MovieVariants(t *testing.T) {
	got := BuildQueries("The Matrix", 1999, 0, 0)
	want := []string{
		"The Matrix 1999",
		"The Matrix 1999 commentary",
		"The Matrix 1999 criterion",
		"The Matrix 1999 special edition",
		"The Matrix 1999 collector",
	}
	set := map[string]bool{}
	for _, q := range got {
		set[q] = true
	}
	for _, w := range want {
		if !set[w] {
			t.Fatalf("missing variant %q in %q", w, got)
		}
	}
}

func TestBuildQueries_NoYear(t *testing.T) {
	got := BuildQueries("Inception", 0, 0, 0)
	for _, q := range got {
		if strings.Contains(q, " 0") {
			t.Fatalf("year 0 should be omitted from query %q", q)
		}
	}
}

func TestBuildQueries_TvShow(t *testing.T) {
	got := BuildQueries("Breaking Bad", 2008, 2, 5)
	found := false
	for _, q := range got {
		if strings.Contains(q, "S02E05") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected at least one query with S02E05, got %q", got)
	}
}

func TestBuildQueries_Deduplicated(t *testing.T) {
	got := BuildQueries("Dup", 2020, 0, 0)
	seen := map[string]bool{}
	for _, q := range got {
		if seen[q] {
			t.Fatalf("duplicate query emitted: %q in %q", q, got)
		}
		seen[q] = true
	}
}
