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
	"syscall"
	"time"

	v1 "github.com/jeffWelling/commentarr/internal/api/v1"
	"github.com/jeffWelling/commentarr/internal/auth"
	"github.com/jeffWelling/commentarr/internal/daemon"
	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/download"
	"github.com/jeffWelling/commentarr/internal/httpserver"
	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/safety"
	"github.com/jeffWelling/commentarr/internal/search"
	"github.com/jeffWelling/commentarr/internal/sse"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/trash"
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
	if err := fset.Parse(args); err != nil {
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

	authRepo := auth.NewRepo(d)
	if err := bootstrapAdmin(authRepo); err != nil {
		return err
	}
	if err := bootstrapAPIKey(authRepo, *initialKeyLabel); err != nil {
		return err
	}

	server := httpserver.New(httpserver.Config{Addr: *addr})
	broker := sse.NewBroker()
	authMW := auth.NewMiddleware(authRepo, auth.MiddlewareConfig{
		LocalBypassCIDRs: splitCIDRs(*bypassCIDR),
	})

	mountAPIV1(server, authMW, d, broker, serveConnections{
		indexers:        infoFromProwlarr(*prowlarrURL, *prowlarrName),
		downloadClients: infoFromQbit(*qbitURL, *qbitName),
	})
	// Serve the embedded React SPA at "/" — the FS falls back to
	// index.html for unknown paths so client-side routing works on
	// hard refresh.
	server.Router().Handle("/*", embeddedSPAHandler())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trashSvc := trash.New(d, trash.Config{Retention: 28 * 24 * time.Hour, AutoPurge: true})

	ticks := []daemon.Tick{
		{Name: "trash-purge", Interval: time.Hour, Fn: func(c context.Context) {
			_, _ = trashSvc.PurgeExpired(c)
		}},
	}
	if tick, ok := buildSearchTick(d, *prowlarrURL, *prowlarrAPIKey, *prowlarrName,
		*prowlarrRPM, *prowlarrBurst, *scoreThreshold, *searchInterval); ok {
		ticks = append(ticks, tick)
		fmt.Printf("search loop enabled: prowlarr=%q, interval=%s\n", *prowlarrName, *searchInterval)
	}
	dmn := daemon.New(daemon.Config{Ticks: ticks})
	go dmn.Run(ctx)

	// Watcher runs its own ticker (it's stateful — dedupes seen jobs),
	// so it lives outside the daemon's Tick set.
	if w, events, ok := buildWatcher(*qbitURL, *qbitUsername, *qbitPassword, *qbitName,
		*watchCategory, *watchInterval); ok {
		go w.Run(ctx, events)
		go logWatcherEvents(ctx, events)
		fmt.Printf("watcher enabled: qbit=%q, category=%q, interval=%s\n", *qbitName, *watchCategory, *watchInterval)
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
// what the daemon was started with.
type serveConnections struct {
	indexers        []v1.IndexerInfo
	downloadClients []v1.DownloadClientInfo
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
	s.Mount("/api/v1/trash", authMW(v1.NewTrashHandler(trashRepo)))
	s.Mount("/api/v1/safety", authMW(v1.NewSafetyHandler(safetyRepo)))
	s.Mount("/api/v1/webhooks", authMW(v1.NewWebhooksHandler(webhookRepo, dispatcher)))

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

// buildWatcher assembles a download.Watcher around a configured qBit
// instance. Returns ok=false when qBit isn't configured (URL empty),
// when the watcher is disabled (interval<=0), or when credentials
// aren't supplied (the qBit API rejects everything without a session).
//
// Returns the watcher and a buffered event channel the caller is
// expected to consume. When the next iteration lands the auto-import
// chain, that consumer becomes the importer route.
func buildWatcher(url, username, password, name, category string, interval time.Duration) (*download.Watcher, chan download.Event, bool) {
	if url == "" || username == "" || password == "" || interval <= 0 {
		return nil, nil, false
	}
	client := download.NewQBittorrent(download.QBittorrentConfig{
		BaseURL: url, Username: username, Password: password, Name: name,
	})
	w := download.NewWatcher([]download.DownloadClient{client}, category, interval)
	// Buffer = 64 lets a brief consumer hiccup absorb several poll cycles
	// without blocking the watcher goroutine. Watcher uses a select on
	// ctx.Done() in its sender, so a stuck consumer eventually unblocks
	// at shutdown.
	events := make(chan download.Event, 64)
	return w, events, true
}

// logWatcherEvents drains the watcher's event channel until ctx is
// cancelled, logging completions + errors. This is the placeholder
// consumer; the next iteration replaces it with the importer routing.
func logWatcherEvents(ctx context.Context, events <-chan download.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			log.Printf("download %s: client=%s job=%s name=%q",
				e.Kind, e.Client, e.Status.ClientJobID, e.Status.Name)
		}
	}
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
