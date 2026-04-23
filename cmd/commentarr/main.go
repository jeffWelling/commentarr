// Command commentarr is the Commentarr binary. Plan 1 ships only the
// "scan" subcommand; later plans add serve, migrate, etc.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jeffWelling/commentarr/internal/classify"
	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/library"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/title"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "scan":
		if err := scan(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  commentarr scan -library <name> -root <path> -db <file>`)
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
