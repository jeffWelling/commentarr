package library

import (
	"context"
	"testing"

	"github.com/jeffWelling/commentarr/internal/title"
)

// fakeSource is an in-test LibrarySource used by contract tests and by
// higher-level integration tests elsewhere in the codebase.
type fakeSource struct {
	name  string
	items []title.Title
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) List(ctx context.Context) ([]title.Title, error) {
	return append([]title.Title(nil), f.items...), nil
}

func (f *fakeSource) Refresh(ctx context.Context, path string) error { return nil }

func TestFakeSource_SatisfiesInterface(t *testing.T) {
	var s LibrarySource = &fakeSource{
		name: "fake",
		items: []title.Title{
			{ID: "x", Kind: title.KindMovie, DisplayName: "X", FilePath: "/x"},
		},
	}
	if s.Name() != "fake" {
		t.Fatalf("Name(): got %q", s.Name())
	}
	got, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].DisplayName != "X" {
		t.Fatalf("unexpected: %+v", got)
	}
	if err := s.Refresh(context.Background(), "/x"); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
}

func TestFakeSource_ListReturnsCopy(t *testing.T) {
	orig := []title.Title{{ID: "a", DisplayName: "A"}}
	f := &fakeSource{items: orig}
	got, err := f.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got[0].DisplayName = "mutated"
	if f.items[0].DisplayName != "A" {
		t.Fatalf("List should return a copy; caller mutation leaked back into source")
	}
}
