package indexer

import "testing"

func TestDeduper_ByInfoHash(t *testing.T) {
	d := NewDeduper()
	a := Release{InfoHash: "ABC123", Title: "Movie A"}
	b := Release{InfoHash: "abc123", Title: "Movie A (duplicate case)"}
	if d.Seen(a) {
		t.Fatal("first occurrence should not be Seen")
	}
	if !d.Seen(b) {
		t.Fatal("case-different infohash should be Seen (we lowercase)")
	}
}

func TestDeduper_ByURL(t *testing.T) {
	d := NewDeduper()
	a := Release{URL: "http://x/y"}
	b := Release{URL: "http://x/y"}
	if d.Seen(a) {
		t.Fatal("first occurrence should not be Seen")
	}
	if !d.Seen(b) {
		t.Fatal("same URL should be Seen")
	}
}

func TestDeduper_IndependentKeys(t *testing.T) {
	d := NewDeduper()
	a := Release{InfoHash: "abc"}
	b := Release{URL: "http://x/y"}
	c := Release{InfoHash: "xyz"}
	if d.Seen(a) || d.Seen(b) || d.Seen(c) {
		t.Fatal("three distinct releases should all pass through")
	}
}

func TestDeduper_FilterReturnsUnique(t *testing.T) {
	d := NewDeduper()
	in := []Release{
		{InfoHash: "ABC"},
		{InfoHash: "abc"},
		{InfoHash: "def"},
		{URL: "http://x/y"},
		{URL: "http://x/y"},
	}
	out := d.Filter(in)
	if len(out) != 3 {
		t.Fatalf("expected 3 unique, got %d: %+v", len(out), out)
	}
}
