// Command commentarr is the Commentarr binary. Plan 1 shipped the scan
// subcommand; Plan 2 adds search. Later plans add serve, migrate, etc.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
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
	case "serve":
		must(serveCmd(os.Args[2:]))
	case "version", "-v", "--version":
		printVersion()
	default:
		usage()
		os.Exit(2)
	}
}

// buildLibrarySource picks the right adapter for the given -source
// flag. Each source has its own required-flag set; missing args fail
// fast with a flag-relative error message.
func buildLibrarySource(source, libName, root, jfURL, jfAPIKey, jfUserID, plexURL, plexToken string) (library.LibrarySource, error) {
	switch source {
	case "filesystem":
		if root == "" {
			return nil, fmt.Errorf("scan: -root is required when -source=filesystem")
		}
		return library.NewFilesystemSource(libName, root), nil
	case "jellyfin", "emby":
		if jfURL == "" || jfAPIKey == "" || jfUserID == "" {
			return nil, fmt.Errorf("scan: -jellyfin-url + -jellyfin-api-key + -jellyfin-user-id are required when -source=%s", source)
		}
		return library.NewJellyfinSource(library.JellyfinConfig{
			BaseURL: jfURL, APIKey: jfAPIKey, UserID: jfUserID, Name: libName,
			EmbyMode: source == "emby",
		}), nil
	case "plex":
		if plexURL == "" || plexToken == "" {
			return nil, fmt.Errorf("scan: -plex-url + -plex-token are required when -source=plex")
		}
		return library.NewPlexSource(library.PlexConfig{
			BaseURL: plexURL, Token: plexToken, Name: libName,
		}), nil
	default:
		return nil, fmt.Errorf("scan: unknown -source=%q (filesystem | jellyfin | emby | plex)", source)
	}
}

// version is overridden at link time via -ldflags '-X main.version=...'.
// "dev" is the in-tree default — release builds set the tag explicitly.
var version = "dev"

func printVersion() {
	fmt.Printf("commentarr %s\n", version)
	if info, ok := debug.ReadBuildInfo(); ok {
		fmt.Printf("  go: %s\n", info.GoVersion)
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				fmt.Printf("  commit: %s\n", s.Value)
			case "vcs.time":
				fmt.Printf("  built: %s\n", s.Value)
			case "GOOS", "GOARCH":
				fmt.Printf("  %s: %s\n", s.Key, s.Value)
			}
		}
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  commentarr scan    -source filesystem -root <path>   -db <file>
                     -source jellyfin -jellyfin-url <url> -jellyfin-api-key <key> -jellyfin-user-id <id>
                     -source plex -plex-url <url> -plex-token <token>
                     [-skip-classify] [-limit N]
  commentarr search  -prowlarr-url <url> -prowlarr-api-key <key> -db <file>
  commentarr import  -new-file <path> -original <path> -title-id <id> -title <name> [-mode sidecar|replace|separate-library]
  commentarr serve   -addr :7878 -db commentarr.db [-local-bypass-cidr 127.0.0.0/8]
  commentarr version`)
}

func scan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	libName := fs.String("library", "local", "library name (used as metric label)")
	source := fs.String("source", "filesystem", "library source: filesystem | jellyfin | emby | plex")
	root := fs.String("root", "", "filesystem root (required when -source=filesystem)")
	jfURL := fs.String("jellyfin-url", "", "Jellyfin/Emby base URL (required when -source=jellyfin or emby)")
	jfAPIKey := fs.String("jellyfin-api-key", "", "Jellyfin/Emby API key or access token")
	jfUserID := fs.String("jellyfin-user-id", "", "Jellyfin/Emby user id (the Items endpoint is user-scoped)")
	plexURL := fs.String("plex-url", "", "Plex base URL (required when -source=plex)")
	plexToken := fs.String("plex-token", "", "Plex token (X-Plex-Token)")
	dsn := fs.String("db", ":memory:", "SQLite DSN")
	migrations := fs.String("migrations", "./migrations", "migrations directory")
	skipClassify := fs.Bool("skip-classify", false, "populate the wanted queue without classifying — useful for first-deploy when the library is large or storage is slow (e.g., over VPN). Every title is marked wanted; subsequent runs without -skip-classify will fill in verdicts.")
	limit := fs.Int("limit", 0, "stop after this many titles (0 = no limit). Useful for first-deploy smoke tests against a real library.")
	if err := fs.Parse(args); err != nil {
		return err
	}

	src, err := buildLibrarySource(*source, *libName, *root,
		*jfURL, *jfAPIKey, *jfUserID, *plexURL, *plexToken)
	if err != nil {
		return err
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

	ctx := context.Background()
	titles, err := src.List(ctx)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	if *limit > 0 && len(titles) > *limit {
		titles = titles[:*limit]
	}

	wanted := 0
	for _, t := range titles {
		if err := repo.Insert(ctx, t); err != nil {
			log.Printf("insert %s: %v", t.ID, err)
			continue
		}
		if *skipClassify {
			if err := q.MarkWanted(ctx, t.ID); err != nil {
				log.Printf("mark wanted %s: %v", t.ID, err)
				continue
			}
			wanted++
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
