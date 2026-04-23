package classify_test

import (
	"context"
	"testing"

	classifier "github.com/jeffWelling/commentary-classifier"
	"github.com/jeffWelling/commentarr/internal/classify"
	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/library"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/title"
)

// fakeClassifier returns commentary verdicts keyed off filepath.
type fakeClassifier map[string][]classifier.TrackResult

func (f fakeClassifier) ClassifyFile(path string) ([]classifier.TrackResult, error) {
	return f[path], nil
}

// TestScan_EndToEnd asserts the full Plan-1 flow:
// 1. Filesystem source enumerates titles from testdata/library/
// 2. Titles are inserted into the title repo
// 3. Classify service produces verdicts
// 4. Wanted queue picks up titles lacking commentary
func TestScan_EndToEnd(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	repo := title.NewRepo(d)
	q := queue.New(d)

	src := library.NewFilesystemSource("fs-test", "../../testdata/library")
	titles, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 2 {
		t.Fatalf("fixture: expected 2 titles, got %d", len(titles))
	}

	for _, tt := range titles {
		if err := repo.Insert(context.Background(), tt); err != nil {
			t.Fatalf("insert %s: %v", tt.ID, err)
		}
	}

	// Stub classifier: the "Movie" file gets commentary, "S01E01" doesn't.
	cls := fakeClassifier{}
	var episodeID string
	for _, tt := range titles {
		if tt.Kind == title.KindMovie {
			cls[tt.FilePath] = []classifier.TrackResult{
				{TrackIndex: 0, Recommendation: "not_commentary", CommentaryConfidence: 0.7},
				{TrackIndex: 1, Recommendation: "commentary", CommentaryConfidence: 0.91},
			}
		} else {
			episodeID = tt.ID
			cls[tt.FilePath] = []classifier.TrackResult{
				{TrackIndex: 0, Recommendation: "not_commentary", CommentaryConfidence: 0.85},
			}
		}
	}
	svc := classify.NewService(repo, cls, "integration-v0")

	for _, tt := range titles {
		v, err := svc.ClassifyTitle(context.Background(), tt)
		if err != nil {
			t.Fatalf("classify %s: %v", tt.ID, err)
		}
		if !v.HasCommentary {
			if err := q.MarkWanted(context.Background(), tt.ID); err != nil {
				t.Fatalf("mark wanted %s: %v", tt.ID, err)
			}
		}
	}

	wanted, err := q.ListByStatus(context.Background(), queue.StatusWanted)
	if err != nil {
		t.Fatal(err)
	}
	if len(wanted) != 1 {
		t.Fatalf("expected exactly 1 wanted (the episode), got %d: %+v", len(wanted), wanted)
	}
	if wanted[0].TitleID != episodeID {
		t.Fatalf("wrong title wanted: %+v (expected %q)", wanted[0], episodeID)
	}
}
