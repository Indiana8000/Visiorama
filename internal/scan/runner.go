package scan

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

// WarmerSuspender is the subset of thumbs.Warmer used by Runner.
type WarmerSuspender interface {
	Suspend()
	Resume()
}

type Runner struct {
	mu     sync.Mutex
	busy   bool
	cfg    *app.Config
	store  *index.Store
	warmer WarmerSuspender
}

func NewRunner(cfg *app.Config, store *index.Store) *Runner {
	return &Runner{cfg: cfg, store: store}
}

// SetWarmer registers the thumb warmer so the runner can suspend it during scans.
func (r *Runner) SetWarmer(w WarmerSuspender) {
	r.warmer = w
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
		if r.warmer != nil {
			r.warmer.Suspend()
		}
		defer func() {
			r.mu.Lock()
			r.busy = false
			r.mu.Unlock()
			if r.warmer != nil {
				r.warmer.Resume()
			}
		}()

		var stats *Stats
		var err error
		fallback := false

		scanRepo := repositories.NewScanRepo(r.store.DB())

		onProgress := func(scanned, indexed, skipped, errors int64) {
			_ = scanRepo.UpdateCounters(scanID,
				int(scanned), int(indexed), int(skipped), int(errors), false)
		}

		switch mode {
		case "full":
			stats, err = NewFullScanner(r.cfg, r.store).RunWithProgress(context.Background(), scanID, onProgress)
		case "quick":
			stats, err, fallback = func() (*Stats, error, bool) {
				s, fb, e := NewQuickScanner(r.cfg, r.store).RunWithProgress(context.Background(), scanID, onProgress)
				return s, e, fb
			}()
		case "orphan":
			stats, err = NewOrphanScanner(r.cfg, r.store).Run(context.Background(), scanID)
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
