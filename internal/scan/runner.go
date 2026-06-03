package scan

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/USERNAME/visiorama/internal/app"
	"github.com/USERNAME/visiorama/internal/index"
	"github.com/USERNAME/visiorama/internal/index/repositories"
)

type Runner struct {
	mu    sync.Mutex
	busy  bool
	cfg   *app.Config
	store *index.Store
}

func NewRunner(cfg *app.Config, store *index.Store) *Runner {
	return &Runner{cfg: cfg, store: store}
}

func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.busy
}

// TriggerAsync enqueues a scan job and runs it in a goroutine.
// Returns an error if a scan is already running.
func (r *Runner) TriggerAsync(scanID, mode string) error {
	r.mu.Lock()
	if r.busy {
		r.mu.Unlock()
		return fmt.Errorf("already running")
	}
	r.busy = true
	r.mu.Unlock()

	scanRepo := repositories.NewScanRepo(r.store.DB())
	startedAt := time.Now().UTC().Format(time.RFC3339)
	_ = scanRepo.UpdateStatus(scanID, "running", &startedAt, nil)

	go func() {
		defer func() {
			r.mu.Lock()
			r.busy = false
			r.mu.Unlock()
		}()

		var stats *Stats
		var err error
		fallback := false

		switch mode {
		case "full":
			stats, err = NewFullScanner(r.cfg, r.store).Run(context.Background(), scanID)
		case "quick":
			stats, err, fallback = func() (*Stats, error, bool) {
				s, fb, e := NewQuickScanner(r.cfg, r.store).Run(context.Background(), scanID)
				return s, e, fb
			}()
		default:
			err = fmt.Errorf("unknown scan mode: %s", mode)
		}

		status := "success"
		if err != nil {
			status = "failed"
			slog.Error("scan failed", "scanId", scanID, "err", err)
		}

		finishedAt := time.Now().UTC().Format(time.RFC3339)
		_ = scanRepo.UpdateStatus(scanID, status, &startedAt, &finishedAt)

		if stats != nil {
			_ = scanRepo.UpdateCounters(scanID,
				int(stats.Scanned.Load()),
				int(stats.Indexed.Load()),
				int(stats.Skipped.Load()),
				int(stats.ErrCount.Load()),
				fallback)
		}
	}()

	return nil
}
