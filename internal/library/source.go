// Package library owns the LibrarySource port — the abstraction every
// library backend (Plex, Jellyfin, Emby, Filesystem) implements.
package library

import (
	"context"

	"github.com/jeffWelling/commentarr/internal/title"
)

// LibrarySource enumerates owned titles in a user's library and can
// request rescans when files change.
type LibrarySource interface {
	// Name is a stable, human-friendly identifier used for log + metric
	// labels. Example: "plex-home", "filesystem-movies".
	Name() string

	// List returns every title the source currently knows about. Paging
	// is not part of v1; backends must materialize the full list in
	// memory. (At 10k titles, one row ≈ 200 bytes, so 2MB total — fine.)
	List(ctx context.Context) ([]title.Title, error)

	// Refresh asks the backend to rescan a path. Best-effort — backends
	// without a rescan API can return nil.
	Refresh(ctx context.Context, path string) error
}
