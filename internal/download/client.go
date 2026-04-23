// Package download holds the DownloadClient port and its v1 qBittorrent
// adapter. The port is small on purpose — add, status, remove. Richer
// features (labels, priority, force-recheck) can be added adapter-side
// without changing the port.
package download

import (
	"context"
	"time"
)

// State enumerates the lifecycle states we care about. Adapters map
// their native vocabulary to these. Unknown/transient states collapse
// to StateOther.
type State string

const (
	StateQueued      State = "queued"
	StateDownloading State = "downloading"
	StateCompleted   State = "completed"
	StatePaused      State = "paused"
	StateError       State = "error"
	StateOther       State = "other"
)

// AddRequest is everything the client needs to enqueue a download.
type AddRequest struct {
	MagnetOrURL string
	Category    string
	SavePath    string
	Paused      bool
}

// Status is a snapshot of a single download.
type Status struct {
	ClientJobID string
	State       State
	Category    string
	SavePath    string
	Name        string
	SizeBytes   int64
	Progress    float64
	CompletedAt time.Time
}

// Completed returns true when the state indicates the torrent finished.
func (s Status) Completed() bool { return s.State == StateCompleted }

// DownloadClient is the port every torrent client adapter implements.
type DownloadClient interface {
	Name() string
	Add(ctx context.Context, req AddRequest) (clientJobID string, err error)
	Status(ctx context.Context, clientJobID string) (Status, error)
	Remove(ctx context.Context, clientJobID string, deleteFiles bool) error
}

// Lister is an optional interface for adapters capable of enumerating
// their current jobs. The watcher uses this to poll for completion
// events without being given every job ID up front.
type Lister interface {
	DownloadClient
	ListByCategory(ctx context.Context, category string) ([]Status, error)
}
