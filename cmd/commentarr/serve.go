package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	v1 "github.com/jeffWelling/commentarr/internal/api/v1"
	"github.com/jeffWelling/commentarr/internal/auth"
	"github.com/jeffWelling/commentarr/internal/classify"
	"github.com/jeffWelling/commentarr/internal/daemon"
	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/download"
	"github.com/jeffWelling/commentarr/internal/httpserver"
	"github.com/jeffWelling/commentarr/internal/importer"
	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/metrics"
	"github.com/jeffWelling/commentarr/internal/placer"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/safety"
	"github.com/jeffWelling/commentarr/internal/search"
	"github.com/jeffWelling/commentarr/internal/sse"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/trash"
	"github.com/jeffWelling/commentarr/internal/validate"
	"github.com/jeffWelling/commentarr/internal/verify"
	"github.com/jeffWelling/commentarr/internal/webhook"
)

func serveCmd(args []string) error {
	fset := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fset.String("addr", ":7878", "HTTP listen address")
	dsn := fset.String("db", "commentarr.db", "SQLite DSN")
	migrations := fset.String("migrations", "./migrations", "migrations directory")
	bypassCIDR := fset.String("local-bypass-cidr", "", "CIDR range that bypasses auth (e.g. 127.0.0.0/8)")
	initialKeyLabel := fset.String("initial-key-label", "default", "label for the auto-generated first API key")
	prowlarrURL := fset.String("prowlarr-url", "", "Prowlarr base URL (optional; shows up in the UI when set)")
	prowlarrAPIKey := fset.String("prowlarr-api-key", "", "Prowlarr API key (required to actually run searches)")
	prowlarrName := fset.String("prowlarr-name", "prowlarr", "Prowlarr instance label")
	prowlarrRPM := fset.Int("prowlarr-rpm", 6, "Prowlarr requests-per-minute rate limit")
	prowlarrBurst := fset.Int("prowlarr-burst", 3, "Prowlarr token-bucket burst")
	searchInterval := fset.Duration("search-interval", 15*time.Minute, "how often the in-process search loop fires (0 disables)")
	scoreThreshold := fset.Int("score-threshold", 8, "release-score threshold for likely-commentary flag")
	qbitURL := fset.String("qbit-url", "", "qBittorrent base URL (optional; shows up in the UI when set)")
	qbitUsername := fset.String("qbit-username", "", "qBittorrent Web UI username (required to actually run the watcher)")
	qbitPassword := fset.String("qbit-password", "", "qBittorrent Web UI password")
	qbitName := fset.String("qbit-name", "qbittorrent", "qBittorrent instance label")
	watchInterval := fset.Duration("watch-interval", 30*time.Second, "how often the in-process watcher polls download clients (0 disables)")
	watchCategory := fset.String("watch-category", "commentarr", "category/label the watcher monitors")
	pickerInterval := fset.Duration("picker-interval", 5*time.Minute, "how often the auto-pick loop runs against wanted titles (0 disables)")
	placementMode := fset.String("placement-mode", "sidecar", "auto-import placement mode: sidecar | replace | separate-library")
	placementTemplate := fset.String("placement-template", "{title} ({year}) - {edition}.{ext}", "auto-import filename template")
	placementSeparate := fset.String("placement-separate-root", "", "alt library root (required when placement-mode=separate-library)")
	placementTrashDir := fset.String("placement-trash-dir", "", "trash directory (required when placement-mode=replace)")
	confidenceMin := fset.Float64("confidence-min", 0.85, "auto-import classifier confidence threshold")
	dryRun := fset.Bool("dry-run", false, "log what the picker + importer would do without queueing downloads or moving files")
	pathTranslateFrom := fset.String("path-translate-from", "", "rewrite this prefix in qBit save paths (e.g. /downloads if qBit reports container paths)")
	pathTranslateTo := fset.String("path-translate-to", "", "...to this prefix on the daemon's filesystem (e.g. /Volumes/downloads for an SMB mount)")
	if err := fset.Parse(args); err != nil {
		return err
	}
	if err := validateServeFlags(*placementMode, *placementSeparate, *placementTrashDir); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	d, err := db.Open(*dsn)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := db.Migrate(d, *migrations); err != nil {
		return err
	}

	authRepo := auth.NewRepo(d)
	if err := bootstrapAdmin(authRepo); err != nil {
		return err
	}
	if err := bootstrapAPIKey(authRepo, *initialKeyLabel); err != nil {
		return err
	}

	server := httpserver.New(httpserver.Config{Addr: *addr})
	server.RegisterReadinessCheck("database", func(c context.Context) error {
		ctx, cancel := context.WithTimeout(c, 2*time.Second)
		defer cancel()
		return d.PingContext(ctx)
	})
	broker := sse.NewBroker()
	authMW := auth.NewMiddleware(authRepo, auth.MiddlewareConfig{
		LocalBypassCIDRs: splitCIDRs(*bypassCIDR),
	})

	mountAPIV1(server, authMW, d, broker, serveConnections{
		indexers:        infoFromProwlarr(*prowlarrURL, *prowlarrName),
		downloadClients: infoFromQbit(*qbitURL, *qbitName),
		startedAt:       time.Now().UTC(),
	})
	// Serve the embedded React SPA at "/" — the FS falls back to
	// index.html for unknown paths so client-side routing works on
	// hard refresh.
	server.Router().Handle("/*", embeddedSPAHandler())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trashSvc := trash.New(d, trash.Config{Retention: 28 * 24 * time.Hour, AutoPurge: true})
	purgeDispatcher := webhook.NewDispatcher(webhook.NewRepo(d), webhook.DispatcherConfig{})

	// Assemble all ticks first; the daemon snapshots the slice on
	// construction, so any append after daemon.New() is silently dropped.
	ticks := []daemon.Tick{
		{Name: "trash-purge", Interval: time.Hour, Fn: func(c context.Context) {
			purged, err := trashSvc.PurgeExpired(c)
			if err != nil {
				log.Printf("trash-purge: %v", err)
				return
			}
			for _, it := range purged {
				_ = purgeDispatcher.Dispatch(c, webhook.EventTrashExpire, map[string]interface{}{
					"library":       it.Library,
					"original_path": it.OriginalPath,
					"trashed_path":  it.TrashPath,
					"reason":        it.Reason,
				})
			}
		}},
	}
	if tick, ok := buildSearchTick(d, *prowlarrURL, *prowlarrAPIKey, *prowlarrName,
		*prowlarrRPM, *prowlarrBurst, *scoreThreshold, *searchInterval); ok {
		ticks = append(ticks, tick)
		fmt.Printf("search loop enabled: prowlarr=%q, interval=%s\n", *prowlarrName, *searchInterval)
	}

	// Build the download client once and share it across the picker
	// tick and the watcher (so both speak to the same logged-in session).
	dlClient, dlOK := buildDownloadClient(*qbitURL, *qbitUsername, *qbitPassword, *qbitName)

	if dlOK && *pickerInterval > 0 {
		ticks = append(ticks, buildPickerTick(d, dlClient, *watchCategory, *scoreThreshold, *pickerInterval, *dryRun))
		mode := ""
		if *dryRun {
			mode = " (dry-run)"
		}
		fmt.Printf("picker enabled%s: interval=%s, threshold=%d\n", mode, *pickerInterval, *scoreThreshold)
	}

	dmn := daemon.New(daemon.Config{Ticks: ticks})
	go dmn.Run(ctx)

	// Watcher + importer consumer run as standalone goroutines because
	// the watcher owns its own ticker (it's stateful — dedupes seen jobs)
	// and the consumer is event-driven, not interval-driven. Under
	// -dry-run we still poll qBit (read-only smoke test) but log events
	// instead of routing them through the importer (which would move
	// files on disk for any pre-existing job rows).
	if dlOK && *watchInterval > 0 {
		events := startWatcher(ctx, dlClient, *watchCategory, *watchInterval)
		if *dryRun {
			go drainEvents(ctx, events)
			fmt.Printf("watcher enabled (dry-run): qbit=%q, category=%q, interval=%s\n",
				*qbitName, *watchCategory, *watchInterval)
		} else {
			go importerConsumer(ctx, d, dlClient, events, placer.Config{
				Mode:             placer.Mode(*placementMode),
				FilenameTemplate: *placementTemplate,
				TrashDir:         *placementTrashDir,
				SeparateRoot:     *placementSeparate,
			}, *confidenceMin, pathTranslator(*pathTranslateFrom, *pathTranslateTo))
			fmt.Printf("watcher+importer enabled: qbit=%q, category=%q, interval=%s\n",
				*qbitName, *watchCategory, *watchInterval)
			if *pathTranslateFrom != "" {
				fmt.Printf("path translate: %q -> %q\n", *pathTranslateFrom, *pathTranslateTo)
			}
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	errCh := make(chan error, 1)
	go func() { errCh <- server.Start() }()

	fmt.Printf("commentarr listening on %s\n", *addr)
	select {
	case <-sigCh:
		fmt.Println("shutting down…")
		cancel()
		return server.Shutdown(context.Background())
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// validateServeFlags catches invalid flag combinations at startup
// instead of letting the importer crash on the first download
// completion. Each placement mode has different prerequisites:
//
//   - replace: must have a trash directory; we move originals there
//     before swapping the new file in.
//   - separate-library: must have an alt root; we copy under it.
//   - sidecar: nothing extra — the new file lives next to the original.
//
// Unknown modes are also a startup-time error.
func validateServeFlags(mode, separateRoot, trashDir string) error {
	switch placer.Mode(mode) {
	case placer.ModeSidecar:
		return nil
	case placer.ModeReplace:
		if trashDir == "" {
			return fmt.Errorf("placement-mode=replace requires -placement-trash-dir")
		}
		return nil
	case placer.ModeSeparateLibrary:
		if separateRoot == "" {
			return fmt.Errorf("placement-mode=separate-library requires -placement-separate-root")
		}
		return nil
	default:
		return fmt.Errorf("placement-mode=%q not recognized (sidecar | replace | separate-library)", mode)
	}
}

// bootstrapAdmin upserts the single admin account from the
// COMMENTARR_ADMIN_USERNAME / COMMENTARR_ADMIN_PASSWORD env vars. Both
// must be set or the call is a no-op — a missing admin is fine; the UI
// can guide the operator through creating one, and API clients use
// API keys regardless.
//
// This is idempotent: restarting the pod with the same values just
// re-hashes and re-stores the password, which is cheap.
func bootstrapAdmin(repo *auth.Repo) error {
	user := os.Getenv("COMMENTARR_ADMIN_USERNAME")
	pass := os.Getenv("COMMENTARR_ADMIN_PASSWORD")
	if user == "" || pass == "" {
		return nil
	}
	hash, err := auth.HashPassword(pass)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	if err := repo.SaveAdmin(context.Background(), user, hash); err != nil {
		return fmt.Errorf("save admin: %w", err)
	}
	return nil
}

// bootstrapAPIKey mints the first API key on fresh installs and prints
// it to stderr so the operator can save it.
func bootstrapAPIKey(repo *auth.Repo, label string) error {
	keys, err := repo.ListAPIKeys(context.Background())
	if err != nil {
		return fmt.Errorf("check API keys: %w", err)
	}
	if len(keys) > 0 {
		return nil
	}
	k, err := repo.GenerateAPIKey(context.Background(), label)
	if err != nil {
		return fmt.Errorf("generate initial API key: %w", err)
	}
	fmt.Fprintf(os.Stderr, "\n=== first-run: API key minted ===\n"+
		"X-Api-Key: %s\n"+
		"(this is the only time the key is printed; save it now)\n\n", k)
	return nil
}

// serveConnections bundles the statically-configured integrations that
// we currently surface read-only in the API. A future iteration will
// replace these with a DB-backed registry that the UI can edit at
// runtime; for now they're populated from flags so an operator can see
// what the daemon was started with. startedAt powers /api/v1/system's
// uptime — captured once at startup, not per request.
type serveConnections struct {
	indexers        []v1.IndexerInfo
	downloadClients []v1.DownloadClientInfo
	startedAt       time.Time
}

func mountAPIV1(s *httpserver.Server, authMW func(http.Handler) http.Handler, d *sql.DB, broker *sse.Broker, conn serveConnections) {
	titleRepo := title.NewRepo(d)
	q := queue.New(d)
	candRepo := search.NewRepo(d)
	trashRepo := trash.NewRepo(d)
	safetyRepo := safety.NewProfileRepo(d)
	webhookRepo := webhook.NewRepo(d)
	dispatcher := webhook.NewDispatcher(webhookRepo, webhook.DispatcherConfig{})

	s.Mount("/api/v1/library", authMW(v1.NewLibraryHandler(titleRepo)))
	s.Mount("/api/v1/wanted", authMW(v1.NewWantedHandler(q, candRepo)))
	s.Mount("/api/v1/indexers", authMW(v1.NewIndexerHandler(conn.indexers)))
	s.Mount("/api/v1/download-clients", authMW(v1.NewDownloadHandler(conn.downloadClients)))
	s.Mount("/api/v1/jobs", authMW(v1.NewJobsHandler(download.NewJobRepo(d))))
	s.Mount("/api/v1/trash", authMW(v1.NewTrashHandler(trashRepo)))
	s.Mount("/api/v1/safety", authMW(v1.NewSafetyHandler(safetyRepo)))
	s.Mount("/api/v1/webhooks", authMW(v1.NewWebhooksHandler(webhookRepo, dispatcher)))
	s.Mount("/api/v1/system", authMW(v1.NewSystemHandler(version, conn.startedAt)))

	s.Router().Mount("/api/v1/events", authMW(sse.NewHandler(broker)))
}

// buildSearchTick assembles a Searcher around a configured Prowlarr
// instance and wraps it as a daemon Tick. Returns ok=false when the
// loop should not run — either Prowlarr isn't configured (URL or key
// missing) or the operator disabled it via interval=0. Logging is
// minimal because each individual indexer call already emits metrics +
// circuit-breaker logs at its own layer.
func buildSearchTick(d *sql.DB, url, apiKey, name string, rpm, burst, threshold int, interval time.Duration) (daemon.Tick, bool) {
	if url == "" || apiKey == "" || interval <= 0 {
		return daemon.Tick{}, false
	}
	rl := indexer.NewRateLimiter(indexer.RateLimitConfig{RequestsPerMinute: rpm, Burst: burst})
	cb := indexer.NewCircuitBreaker(indexer.CircuitBreakerConfig{
		ConsecutiveFailureThreshold: 5,
		OpenDuration:                time.Hour,
	})
	idx := indexer.NewProwlarr(indexer.ProwlarrConfig{
		BaseURL: url, APIKey: apiKey, Name: name,
	}, rl, cb)
	searcher := search.NewSearcher(
		[]indexer.Indexer{idx},
		verify.NewVerifier(verify.DefaultRules(), threshold),
		search.NewRepo(d),
		queue.New(d),
		title.NewRepo(d),
		100,
	)
	return daemon.Tick{
		Name:     "search-due",
		Interval: interval,
		Fn: func(c context.Context) {
			n, err := searcher.SearchDue(c, time.Now())
			if err != nil {
				log.Printf("search tick: %v", err)
				return
			}
			if n > 0 {
				log.Printf("search tick: processed %d titles", n)
			}
		},
	}, true
}

// buildDownloadClient assembles a qBit adapter from flags. Returns
// (nil, false) when qBit isn't fully configured — both URL and creds
// are required (the qBit API rejects unauthenticated calls).
func buildDownloadClient(url, username, password, name string) (download.DownloadClient, bool) {
	if url == "" || username == "" || password == "" {
		return nil, false
	}
	return download.NewQBittorrent(download.QBittorrentConfig{
		BaseURL: url, Username: username, Password: password, Name: name,
	}), true
}

// startWatcher attaches a download.Watcher to the given client and
// starts its poll loop. The caller is responsible for honoring
// "interval > 0" before calling — a non-positive interval here would
// produce a Watcher whose underlying ticker silently uses the
// default fallback (5s in download.NewWatcher), which would surprise
// the operator. The check lives at the call site.
func startWatcher(ctx context.Context, client download.DownloadClient, category string, interval time.Duration) chan download.Event {
	w := download.NewWatcher([]download.DownloadClient{client}, category, interval)
	// Buffer = 64 lets a brief consumer hiccup absorb several poll cycles
	// without blocking the watcher goroutine. Watcher uses a select on
	// ctx.Done() in its sender, so a stuck consumer eventually unblocks
	// at shutdown.
	events := make(chan download.Event, 64)
	go w.Run(ctx, events)
	return events
}

// buildPickerTick returns a daemon Tick that walks the wanted queue
// every interval and queues the top likely-commentary candidate per
// title via the Picker. Eligibility checks (existing in-flight job,
// score threshold) live inside Picker.PickAndQueueOne.
func buildPickerTick(d *sql.DB, client download.DownloadClient, category string, threshold int, interval time.Duration, dryRun bool) daemon.Tick {
	picker := search.NewPicker(search.NewRepo(d), download.NewJobRepo(d), client,
		webhook.NewDispatcher(webhook.NewRepo(d), webhook.DispatcherConfig{}),
		category, threshold).WithDryRun(dryRun)
	q := queue.New(d)
	return daemon.Tick{
		Name:     "picker",
		Interval: interval,
		Fn: func(c context.Context) {
			wanted, err := q.ListByStatus(c, queue.StatusWanted)
			if err != nil {
				log.Printf("picker tick: list wanted: %v", err)
				return
			}
			queued := 0
			for _, e := range wanted {
				_, ok, perr := picker.PickAndQueueOne(c, e.TitleID)
				if perr != nil {
					log.Printf("picker tick: %s: %v", e.TitleID, perr)
					continue
				}
				if ok {
					queued++
				}
			}
			if queued > 0 {
				log.Printf("picker tick: queued %d downloads", queued)
			}
		},
	}
}

// drainEvents replaces importerConsumer in dry-run mode. It just logs
// each event the watcher emits so the operator can see qBit is being
// polled, then drops the event. No DB lookups, no file moves, no
// importer invocations.
func drainEvents(ctx context.Context, events <-chan download.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			log.Printf("DRY-RUN: watcher saw %s for client=%s job=%s name=%q (would route to importer if real run)",
				e.Kind, e.Client, e.Status.ClientJobID, e.Status.Name)
		}
	}
}

// importerConsumer routes Watcher completions through the full import
// pipeline. For each completed event, it looks up the corresponding
// download_jobs row to get the title id, loads the title, finds the
// largest video file under the client's reported SavePath, and runs
// importer.Import(). The job row is then marked imported / error
// based on the outcome.
//
// Errors during routing (job not found, title not found, etc) are
// logged — they're recoverable across restarts because the watcher
// dedupe set is in-memory only, so a restart re-emits the same
// completion event.
// pathTranslator returns a func that rewrites the configured prefix in
// a path. When from is empty, the returned func is the identity. Used
// to bridge "qBit reports container paths, daemon sees a mounted
// filesystem at a different mount-point" deployments — e.g., daemon
// runs on a Mac with /Volumes/downloads SMB-mounted while qBit
// reports /downloads.
func pathTranslator(from, to string) func(string) string {
	if from == "" {
		return func(s string) string { return s }
	}
	return func(s string) string {
		if strings.HasPrefix(s, from) {
			return to + s[len(from):]
		}
		return s
	}
}

func importerConsumer(ctx context.Context, d *sql.DB, client download.DownloadClient, events <-chan download.Event, placeCfg placer.Config, confidenceMin float64, translatePath func(string) string) {
	jobs := download.NewJobRepo(d)
	titles := title.NewRepo(d)
	q := queue.New(d)
	pl := placer.New(placeCfg)
	cls := classify.NewService(titles, classify.NewPipelineClassifier(), "commentarr-serve", client.Name())
	tr := trash.New(d, trash.Config{Retention: 28 * 24 * time.Hour, AutoPurge: true})
	disp := webhook.NewDispatcher(webhook.NewRepo(d), webhook.DispatcherConfig{})
	imp := importer.New(importer.Deps{
		Classify: cls, Placer: pl, Trash: tr, Webhook: disp,
		SafetyCfg: safety.BuiltinConfig{
			ClassifierConfidenceThreshold: confidenceMin,
			RequireMagicMatch:             true,
		},
		Library: client.Name(),
	})
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			e.Status.SavePath = translatePath(e.Status.SavePath)
			// OnDownload fires for every terminal event (completed +
			// error) — receivers want to know about failed downloads
			// too, not just successes. Importer fires its own OnImport
			// after a successful place; that's downstream of this.
			_ = disp.Dispatch(ctx, webhook.EventDownload, map[string]interface{}{
				"client":        e.Client,
				"client_job_id": e.Status.ClientJobID,
				"kind":          string(e.Kind),
				"name":          e.Status.Name,
				"save_path":     e.Status.SavePath,
			})
			handleEvent(ctx, jobs, titles, q, imp, e)
		}
	}
}

// importRunner abstracts the slice of *importer.Importer that
// handleEvent actually uses. Lets tests inject a stub without standing
// up the full classify/placer/trash/webhook stack.
type importRunner interface {
	Import(ctx context.Context, req importer.Request) (importer.Result, error)
}

func handleEvent(ctx context.Context, jobs *download.JobRepo, titles title.Repo, q *queue.Queue, imp importRunner, e download.Event) {
	metrics.WatcherEventsTotal.WithLabelValues(e.Client, string(e.Kind)).Inc()
	if e.Kind != download.EventCompleted {
		log.Printf("download %s: client=%s job=%s", e.Kind, e.Client, e.Status.ClientJobID)
		return
	}
	job, err := jobs.FindByClientJob(ctx, e.Client, e.Status.ClientJobID)
	if err != nil {
		metrics.AutoImportRoutingErrorsTotal.WithLabelValues("job_not_found").Inc()
		log.Printf("import: lookup job (%s/%s): %v", e.Client, e.Status.ClientJobID, err)
		return
	}
	if job.Status == "imported" {
		return
	}
	t, err := titles.FindByID(ctx, job.TitleID)
	if err != nil {
		metrics.AutoImportRoutingErrorsTotal.WithLabelValues("title_not_found").Inc()
		log.Printf("import: lookup title %s: %v", job.TitleID, err)
		_ = jobs.MarkStatus(ctx, job.ID, "error", "title not found")
		return
	}
	newPath, err := validate.FindMainVideo(e.Status.SavePath)
	if err != nil {
		metrics.AutoImportRoutingErrorsTotal.WithLabelValues("no_main_video").Inc()
		log.Printf("import: find main video in %q: %v", e.Status.SavePath, err)
		_ = jobs.MarkStatus(ctx, job.ID, "error", err.Error())
		return
	}
	res, err := imp.Import(ctx, importer.Request{
		NewFilePath:      newPath,
		OriginalFilePath: t.FilePath,
		TitleID:          t.ID,
		Title:            t.DisplayName,
		Year:             yearOf(t),
		Edition:          job.Edition,
	})
	if err != nil {
		metrics.AutoImportRoutingErrorsTotal.WithLabelValues("import_error").Inc()
		log.Printf("import: %s: %v", t.ID, err)
		_ = jobs.MarkStatus(ctx, job.ID, "error", err.Error())
		return
	}
	_ = jobs.MarkStatus(ctx, job.ID, "imported", string(res.Outcome))
	// Only an OutcomeSuccess means the new commentary-bearing file is
	// now part of the library — that's when the title transitions out
	// of "wanted." Other outcomes (safety_violation, non_compliant,
	// error) leave the title wanted so the next search cycle can find
	// a better candidate.
	if res.Outcome == importer.OutcomeSuccess {
		if err := q.MarkResolved(ctx, t.ID); err != nil {
			log.Printf("import: MarkResolved %s: %v", t.ID, err)
		}
	}
	log.Printf("import: %s -> %s (outcome=%s)", t.ID, res.FinalPath, res.Outcome)
}

// yearOf renders a title's year as a string, returning "" when unknown
// (zero year). The importer's filename template handles empty year
// substitution gracefully.
func yearOf(t title.Title) string {
	if t.Year == 0 {
		return ""
	}
	return fmt.Sprintf("%d", t.Year)
}

// infoFromProwlarr builds an IndexerInfo slice for a configured Prowlarr
// instance (or returns nil if the URL is empty).
func infoFromProwlarr(baseURL, name string) []v1.IndexerInfo {
	if baseURL == "" {
		return nil
	}
	return []v1.IndexerInfo{{Name: name, Kind: "prowlarr", BaseURL: baseURL, Enabled: true}}
}

// infoFromQbit builds a DownloadClientInfo slice for a configured
// qBittorrent instance (or returns nil if the URL is empty).
func infoFromQbit(baseURL, name string) []v1.DownloadClientInfo {
	if baseURL == "" {
		return nil
	}
	return []v1.DownloadClientInfo{{Name: name, Kind: "qbittorrent", BaseURL: baseURL, Enabled: true}}
}

func splitCIDRs(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
