// Command commentarr is the Commentarr binary. Plan 1 shipped the scan
// subcommand; Plan 2 adds search. Later plans add serve, migrate, etc.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jeffWelling/commentarr/internal/classify"
	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/importer"
	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/library"
	"github.com/jeffWelling/commentarr/internal/placer"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/safety"
	"github.com/jeffWelling/commentarr/internal/search"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/trash"
	"github.com/jeffWelling/commentarr/internal/verify"
	"github.com/jeffWelling/commentarr/internal/webhook"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "scan":
		must(scan(os.Args[2:]))
	case "search":
		must(searchCmd(os.Args[2:]))
	case "import":
		must(importCmd(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  commentarr scan   -root <path>   -db <file>
  commentarr search -prowlarr-url <url> -prowlarr-api-key <key> -db <file>
  commentarr import -new-file <path> -original <path> -title-id <id> -title <name> [-mode sidecar|replace|separate-library]`)
}

func scan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	libName := fs.String("library", "local", "library name (used as metric label)")
	root := fs.String("root", "", "filesystem root to scan (required)")
	dsn := fs.String("db", ":memory:", "SQLite DSN")
	migrations := fs.String("migrations", "./migrations", "migrations directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("scan: -root is required")
	}

	d, err := db.Open(*dsn)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := db.Migrate(d, *migrations); err != nil {
		return err
	}

	repo := title.NewRepo(d)
	q := queue.New(d)
	cls := classify.NewPipelineClassifier()
	svc := classify.NewService(repo, cls, "commentarr-plan1", *libName)
	src := library.NewFilesystemSource(*libName, *root)

	ctx := context.Background()
	titles, err := src.List(ctx)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}

	wanted := 0
	for _, t := range titles {
		if err := repo.Insert(ctx, t); err != nil {
			log.Printf("insert %s: %v", t.ID, err)
			continue
		}
		v, err := svc.ClassifyTitle(ctx, t)
		if err != nil {
			log.Printf("classify %s: %v", t.ID, err)
			continue
		}
		if !v.HasCommentary {
			if err := q.MarkWanted(ctx, t.ID); err != nil {
				log.Printf("mark wanted %s: %v", t.ID, err)
				continue
			}
			wanted++
		}
	}

	fmt.Printf("Scanned %d titles from %q; %d wanted (no commentary found).\n",
		len(titles), *libName, wanted)
	return nil
}

func searchCmd(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	baseURL := fs.String("prowlarr-url", "", "Prowlarr base URL (required)")
	apiKey := fs.String("prowlarr-api-key", "", "Prowlarr API key (required)")
	name := fs.String("prowlarr-name", "prowlarr", "Prowlarr indexer label")
	rpm := fs.Int("requests-per-minute", 6, "Prowlarr rate limit")
	burst := fs.Int("burst", 3, "Prowlarr burst")
	threshold := fs.Int("score-threshold", 8, "release-score threshold for likely-commentary flag")
	dsn := fs.String("db", ":memory:", "SQLite DSN")
	migrations := fs.String("migrations", "./migrations", "migrations directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baseURL == "" || *apiKey == "" {
		return fmt.Errorf("search: -prowlarr-url and -prowlarr-api-key are required")
	}

	d, err := db.Open(*dsn)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := db.Migrate(d, *migrations); err != nil {
		return err
	}

	rl := indexer.NewRateLimiter(indexer.RateLimitConfig{RequestsPerMinute: *rpm, Burst: *burst})
	cb := indexer.NewCircuitBreaker(indexer.CircuitBreakerConfig{
		ConsecutiveFailureThreshold: 5,
		OpenDuration:                time.Hour,
	})
	idx := indexer.NewProwlarr(indexer.ProwlarrConfig{
		BaseURL: *baseURL, APIKey: *apiKey, Name: *name,
	}, rl, cb)

	searcher := search.NewSearcher(
		[]indexer.Indexer{idx},
		verify.NewVerifier(verify.DefaultRules(), *threshold),
		search.NewRepo(d),
		queue.New(d),
		title.NewRepo(d),
		100,
	)

	ctx := context.Background()
	n, err := searcher.SearchDue(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	fmt.Printf("Searched %d title(s).\n", n)
	return nil
}

func importCmd(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	newPath := fs.String("new-file", "", "downloaded file to import (required)")
	origPath := fs.String("original", "", "original file to replace / live beside (required for replace/sidecar)")
	titleID := fs.String("title-id", "", "title id (required)")
	titleName := fs.String("title", "", "display title (required)")
	year := fs.String("year", "", "release year")
	edition := fs.String("edition", "", "edition label (e.g. Criterion)")
	mode := fs.String("mode", "sidecar", "placement mode: replace | sidecar | separate-library")
	libName := fs.String("library", "local", "library label")
	trashDir := fs.String("trash", "", "trash directory (required for replace)")
	separateRoot := fs.String("separate-root", "", "alt library root (required for separate-library)")
	template := fs.String("template", "{title} ({year}) - {edition}.{ext}", "filename template")
	dsn := fs.String("db", ":memory:", "SQLite DSN")
	migrations := fs.String("migrations", "./migrations", "migrations directory")
	confidenceMin := fs.Float64("confidence-min", 0.85, "classifier confidence threshold")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *newPath == "" || *titleID == "" || *titleName == "" {
		return fmt.Errorf("import: -new-file, -title-id, -title are required")
	}

	d, err := db.Open(*dsn)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := db.Migrate(d, *migrations); err != nil {
		return err
	}

	pl := placer.New(placer.Config{
		Mode:             placer.Mode(*mode),
		FilenameTemplate: *template,
		TrashDir:         *trashDir,
		SeparateRoot:     *separateRoot,
	})
	repo := title.NewRepo(d)
	_ = repo.Insert(context.Background(), title.Title{
		ID:          *titleID,
		Kind:        title.KindMovie,
		DisplayName: *titleName,
		FilePath:    *newPath,
	})
	cls := classify.NewService(repo, classify.NewPipelineClassifier(), "commentarr-plan3", *libName)
	tr := trash.New(d, trash.Config{Retention: 28 * 24 * time.Hour, AutoPurge: true})
	disp := webhook.NewDispatcher(webhook.NewRepo(d), webhook.DispatcherConfig{})

	imp := importer.New(importer.Deps{
		Classify: cls, Placer: pl, Trash: tr, Webhook: disp,
		SafetyCfg: safety.BuiltinConfig{
			ClassifierConfidenceThreshold: *confidenceMin,
			RequireMagicMatch:             true,
		},
		Library: *libName,
	})
	res, err := imp.Import(context.Background(), importer.Request{
		NewFilePath: *newPath, OriginalFilePath: *origPath,
		Title: *titleName, Year: *year, Edition: *edition, TitleID: *titleID,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Outcome: %s  Final: %s  Trashed: %s\n", res.Outcome, res.FinalPath, res.TrashedPath)
	return nil
}
