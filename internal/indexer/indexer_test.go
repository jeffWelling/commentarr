package indexer

import (
	"context"
	"testing"
)

// fakeIndexer is a minimal Indexer used to exercise the interface.
type fakeIndexer struct {
	name     string
	releases []Release
}

func (f *fakeIndexer) Name() string { return f.name }

func (f *fakeIndexer) Search(ctx context.Context, q Query) ([]Release, error) {
	return append([]Release(nil), f.releases...), nil
}

func TestFakeIndexer_SatisfiesInterface(t *testing.T) {
	var i Indexer = &fakeIndexer{
		name: "fake",
		releases: []Release{
			{InfoHash: "abc", Title: "Movie 2020 Criterion", SizeBytes: 1 << 30, Seeders: 5, Indexer: "fake"},
		},
	}
	if i.Name() != "fake" {
		t.Fatalf("Name(): got %q", i.Name())
	}
	got, err := i.Search(context.Background(), Query{Title: "Movie", Year: 2020})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Movie 2020 Criterion" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRelease_Identity(t *testing.T) {
	r := Release{InfoHash: "ABC123", URL: "http://x/y"}
	if r.Identity() != "infohash:abc123" {
		t.Fatalf("InfoHash should be lowercase-prefixed: got %q", r.Identity())
	}
	r2 := Release{URL: "http://example.com/dl?token=xyz"}
	if r2.Identity() != "url:http://example.com/dl?token=xyz" {
		t.Fatalf("URL fallback: got %q", r2.Identity())
	}
	r3 := Release{Indexer: "x", Title: "y"}
	if r3.Identity() == "" {
		t.Fatal("empty Release should still produce a stable identity string (title/indexer fallback)")
	}
}
