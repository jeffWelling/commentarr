// Package daemon runs the background loops (searcher, watcher, trash
// purge, etc.) that would otherwise need one goroutine-per-loop
// scattered across packages. One Daemon, one Run loop, uniform shutdown
// semantics.
package daemon

import (
	"context"
	"sync"
	"time"
)

// Tick is one periodic job.
type Tick struct {
	Name     string
	Interval time.Duration
	Fn       func(ctx context.Context)
}

// Config bundles a set of Ticks.
type Config struct {
	Ticks []Tick
}

// Daemon runs every configured Tick in its own goroutine until ctx is
// cancelled.
type Daemon struct {
	cfg Config
}

// New returns a Daemon.
func New(cfg Config) *Daemon { return &Daemon{cfg: cfg} }

// Run blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, t := range d.cfg.Ticks {
		wg.Add(1)
		go func(t Tick) {
			defer wg.Done()
			tick := time.NewTicker(t.Interval)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					t.Fn(ctx)
				}
			}
		}(t)
	}
	wg.Wait()
}
