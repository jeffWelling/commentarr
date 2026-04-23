package importer_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	classifier "github.com/jeffWelling/commentary-classifier"
	"github.com/jeffWelling/commentarr/internal/classify"
	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/importer"
	"github.com/jeffWelling/commentarr/internal/placer"
	"github.com/jeffWelling/commentarr/internal/safety"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/trash"
	"github.com/jeffWelling/commentarr/internal/webhook"
)

var mkvMagic = []byte{
	0x1A, 0x45, 0xDF, 0xA3,
	0xA3, 0x42, 0x82, 0x88,
	'm', 'a', 't', 'r', 'o', 's', 'k', 'a',
}

type stubClassifier struct{}

func (stubClassifier) ClassifyFile(path string) ([]classifier.TrackResult, error) {
	return []classifier.TrackResult{
		{TrackIndex: 1, Recommendation: "commentary", CommentaryConfidence: 0.95},
	}, nil
}

func TestImport_EndToEnd(t *testing.T) {
	ctx := context.Background()

	var received []webhook.Envelope
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env webhook.Envelope
		_ = json.Unmarshal(body, &env)
		mu.Lock()
		received = append(received, env)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	wrepo := webhook.NewRepo(d)
	_ = wrepo.SaveSubscriber(ctx, webhook.Subscriber{
		ID: "s1", Name: "test", URL: server.URL, Enabled: true,
		Events: []webhook.Event{webhook.EventImport, webhook.EventTrash, webhook.EventReplace},
	})

	tmp := t.TempDir()
	libDir := filepath.Join(tmp, "library", "Movie (2020)")
	trashDir := filepath.Join(tmp, "trash")
	origPath := filepath.Join(libDir, "Movie (2020).mkv")
	newPath := filepath.Join(tmp, "download", "Movie.2020.Criterion.mkv")
	_ = os.MkdirAll(libDir, 0o755)
	_ = os.MkdirAll(filepath.Dir(newPath), 0o755)
	_ = os.WriteFile(origPath, append(mkvMagic, make([]byte, 1024)...), 0o644)
	_ = os.WriteFile(newPath, append(mkvMagic, make([]byte, 2048)...), 0o644)

	// Pre-insert the title so Classify.ClassifyTitle can SaveVerdict.
	titleRepo := title.NewRepo(d)
	if err := titleRepo.Insert(ctx, title.Title{
		ID: "t:e2e", Kind: title.KindMovie, DisplayName: "Movie", Year: 2020, FilePath: newPath,
	}); err != nil {
		t.Fatal(err)
	}

	pl := placer.New(placer.Config{
		Mode:             placer.ModeReplace,
		FilenameTemplate: "{title} ({year}).{ext}",
		TrashDir:         trashDir,
	})
	cls := classify.NewService(titleRepo, stubClassifier{}, "e2e-v0", "lib1")
	tr := trash.New(d, trash.Config{Retention: time.Hour, AutoPurge: true})
	disp := webhook.NewDispatcher(wrepo, webhook.DispatcherConfig{MaxAttempts: 2, RetryBackoff: time.Millisecond})

	imp := importer.New(importer.Deps{
		Classify: cls, Placer: pl, Trash: tr, Webhook: disp,
		SafetyCfg: safety.BuiltinConfig{
			ClassifierConfidenceThreshold: 0.85,
			RequireMagicMatch:             true,
		},
		Library: "lib1",
	})

	res, err := imp.Import(ctx, importer.Request{
		NewFilePath: newPath, OriginalFilePath: origPath,
		Title: "Movie", Year: "2020", Edition: "Criterion", TitleID: "t:e2e",
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Outcome != importer.OutcomeSuccess {
		t.Fatalf("expected success, got %+v", res)
	}
	if res.FinalPath != origPath {
		t.Fatalf("replace should have landed new file at origPath; got %s", res.FinalPath)
	}
	if _, err := os.Stat(res.TrashedPath); err != nil {
		t.Fatalf("trashed file should exist at %s: %v", res.TrashedPath, err)
	}

	mu.Lock()
	defer mu.Unlock()
	seen := map[webhook.Event]bool{}
	for _, env := range received {
		seen[env.EventType] = true
	}
	for _, e := range []webhook.Event{webhook.EventImport, webhook.EventTrash, webhook.EventReplace} {
		if !seen[e] {
			t.Errorf("expected event %s not received; got %+v", e, received)
		}
	}
}
