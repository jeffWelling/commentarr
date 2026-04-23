package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	v1 "github.com/jeffWelling/commentarr/internal/api/v1"
	"github.com/jeffWelling/commentarr/internal/auth"
	"github.com/jeffWelling/commentarr/internal/daemon"
	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/httpserver"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/safety"
	"github.com/jeffWelling/commentarr/internal/search"
	"github.com/jeffWelling/commentarr/internal/sse"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/trash"
	"github.com/jeffWelling/commentarr/internal/webhook"
)

func serveCmd(args []string) error {
	fset := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fset.String("addr", ":7878", "HTTP listen address")
	dsn := fset.String("db", "commentarr.db", "SQLite DSN")
	migrations := fset.String("migrations", "./migrations", "migrations directory")
	bypassCIDR := fset.String("local-bypass-cidr", "", "CIDR range that bypasses auth (e.g. 127.0.0.0/8)")
	initialKeyLabel := fset.String("initial-key-label", "default", "label for the auto-generated first API key")
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
	if err := bootstrapAPIKey(authRepo, *initialKeyLabel); err != nil {
		return err
	}

	server := httpserver.New(httpserver.Config{Addr: *addr})
	broker := sse.NewBroker()
	authMW := auth.NewMiddleware(authRepo, auth.MiddlewareConfig{
		LocalBypassCIDRs: splitCIDRs(*bypassCIDR),
	})

	mountAPIV1(server, authMW, d, broker)
	server.Mount("/", spaHandler())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trashSvc := trash.New(d, trash.Config{Retention: 28 * 24 * time.Hour, AutoPurge: true})
	dmn := daemon.New(daemon.Config{
		Ticks: []daemon.Tick{
			{Name: "trash-purge", Interval: time.Hour, Fn: func(c context.Context) {
				_, _ = trashSvc.PurgeExpired(c)
			}},
		},
	})
	go dmn.Run(ctx)

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

func mountAPIV1(s *httpserver.Server, authMW func(http.Handler) http.Handler, d *sql.DB, broker *sse.Broker) {
	titleRepo := title.NewRepo(d)
	q := queue.New(d)
	candRepo := search.NewRepo(d)
	trashRepo := trash.NewRepo(d)
	safetyRepo := safety.NewProfileRepo(d)
	webhookRepo := webhook.NewRepo(d)
	dispatcher := webhook.NewDispatcher(webhookRepo, webhook.DispatcherConfig{})

	s.Mount("/api/v1/library", authMW(v1.NewLibraryHandler(titleRepo)))
	s.Mount("/api/v1/wanted", authMW(v1.NewWantedHandler(q, candRepo)))
	s.Mount("/api/v1/indexers", authMW(v1.NewIndexerHandler(nil)))        // Plan 4 scope: empty list; wire in Plan 5
	s.Mount("/api/v1/download-clients", authMW(v1.NewDownloadHandler(nil))) // same
	s.Mount("/api/v1/trash", authMW(v1.NewTrashHandler(trashRepo)))
	s.Mount("/api/v1/safety", authMW(v1.NewSafetyHandler(safetyRepo)))
	s.Mount("/api/v1/webhooks", authMW(v1.NewWebhooksHandler(webhookRepo, dispatcher)))

	s.Router().Mount("/api/v1/events", authMW(sse.NewHandler(broker)))
}

func spaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "web UI not built — run `cd web && npm run build`", http.StatusNotImplemented)
	})
}

func splitCIDRs(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
