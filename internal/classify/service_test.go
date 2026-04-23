package classify

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	classifier "github.com/jeffWelling/commentary-classifier"
	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/title"
)

type stubFn func(path string) ([]classifier.TrackResult, error)

func (s stubFn) ClassifyFile(path string) ([]classifier.TrackResult, error) { return s(path) }

func newServiceWithStub(t *testing.T, fn stubFn) (*Service, title.Repo) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	repo := title.NewRepo(d)
	return NewService(repo, fn, "test-v0"), repo
}

func TestService_ClassifyTitle_CommentaryDetected(t *testing.T) {
	svc, repo := newServiceWithStub(t, func(path string) ([]classifier.TrackResult, error) {
		return []classifier.TrackResult{
			{TrackIndex: 0, Recommendation: "not_commentary", CommentaryConfidence: 0.8},
			{TrackIndex: 1, Recommendation: "commentary", CommentaryConfidence: 0.95},
		}, nil
	})
	tt := title.Title{ID: "t:1", Kind: title.KindMovie, DisplayName: "X", FilePath: "/x.mkv"}
	if err := repo.Insert(context.Background(), tt); err != nil {
		t.Fatal(err)
	}

	verdict, err := svc.ClassifyTitle(context.Background(), tt)
	if err != nil {
		t.Fatalf("ClassifyTitle: %v", err)
	}
	if !verdict.HasCommentary {
		t.Fatal("expected HasCommentary=true")
	}
	if verdict.Confidence != 0.95 {
		t.Fatalf("expected confidence 0.95 (max of commentary tracks), got %f", verdict.Confidence)
	}

	saved, err := repo.GetVerdict(context.Background(), "t:1")
	if err != nil {
		t.Fatal(err)
	}
	if !saved.HasCommentary {
		t.Fatalf("saved verdict should have HasCommentary=true, got %+v", saved)
	}
	if time.Since(saved.ClassifiedAt) > time.Minute {
		t.Fatalf("classified_at too old: %v", saved.ClassifiedAt)
	}
}

func TestService_ClassifyTitle_NoCommentary(t *testing.T) {
	svc, repo := newServiceWithStub(t, func(path string) ([]classifier.TrackResult, error) {
		return []classifier.TrackResult{
			{TrackIndex: 0, Recommendation: "not_commentary", CommentaryConfidence: 0.88},
		}, nil
	})
	tt := title.Title{ID: "t:2", Kind: title.KindMovie, DisplayName: "Y", FilePath: "/y.mkv"}
	if err := repo.Insert(context.Background(), tt); err != nil {
		t.Fatal(err)
	}
	v, err := svc.ClassifyTitle(context.Background(), tt)
	if err != nil {
		t.Fatal(err)
	}
	if v.HasCommentary {
		t.Fatal("expected HasCommentary=false")
	}
	if v.Confidence != 0.88 {
		t.Fatalf("expected 0.88 got %f", v.Confidence)
	}
}

func TestPipelineClassifier_RejectsMissingFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	pc := NewPipelineClassifier()
	if _, err := pc.ClassifyFile("/definitely/not/a/real/file.mkv"); err == nil {
		t.Fatal("expected error for bogus path")
	}
}

func TestService_ClassifyTitle_ClassifierError(t *testing.T) {
	svc, repo := newServiceWithStub(t, func(path string) ([]classifier.TrackResult, error) {
		return nil, errors.New("ffmpeg died")
	})
	tt := title.Title{ID: "t:3", Kind: title.KindMovie, DisplayName: "Z", FilePath: "/z.mkv"}
	if err := repo.Insert(context.Background(), tt); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ClassifyTitle(context.Background(), tt)
	if err == nil {
		t.Fatal("expected error")
	}
}
