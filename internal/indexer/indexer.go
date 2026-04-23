package indexer

import "context"

// Indexer is the port every search backend (Prowlarr today; Jackett /
// raw Torznab later) implements. Implementations must be safe for
// concurrent callers.
type Indexer interface {
	// Name identifies the backend in logs + metric labels.
	Name() string

	// Search issues a single search. Rate-limiting + circuit-breaking
	// happen inside the implementation; callers need not pre-wait.
	Search(ctx context.Context, q Query) ([]Release, error)
}
