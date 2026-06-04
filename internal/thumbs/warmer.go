package thumbs

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/util"
)

const (
	warmerItemDelay  = 250 * time.Millisecond
	warmerResumeWait = 30 * time.Second
)

// MediaSource is the subset of MediaRepo used by Warmer.
type MediaSource interface {
	NextThumbPending() (*repositories.Media, error)
	CountThumbPending() (int, error)
	SetThumbReady(id int64, ready bool) error
}

// Warmer pre-generates thumbnails for all media items that have thumb_ready=0.
// It runs as a single background goroutine to stay resource-friendly.
// Foreground thumbnail requests call Pause() to yield the CPU for 30 seconds.
type Warmer struct {
	media         MediaSource
	rootPath      string
	cacheDir      string
	defaultWidth  int
	defaultHeight int

	paused  atomic.Int64 // unix-nano of last Pause() call; 0 = not paused
	pending atomic.Int64
	running atomic.Bool
}

func NewWarmer(media MediaSource, rootPath, cacheDir string, defaultWidth, defaultHeight int) *Warmer {
	return &Warmer{
		media:         media,
		rootPath:      rootPath,
		cacheDir:      cacheDir,
		defaultWidth:  defaultWidth,
		defaultHeight: defaultHeight,
	}
}

// Pause signals the warmer to yield for warmerResumeWait.
// Called by the foreground thumbnail handler on every cache miss.
func (w *Warmer) Pause() {
	w.paused.Store(time.Now().UnixNano())
}

// Pending returns the last known count of items still needing thumbnails.
func (w *Warmer) Pending() int64 {
	return w.pending.Load()
}

// Running reports whether the warmer goroutine is currently generating thumbnails.
func (w *Warmer) Running() bool {
	return w.running.Load()
}

// Start launches the background warmer goroutine. Safe to call at server boot.
// Does nothing if no items need warming.
func (w *Warmer) Start(ctx context.Context) {
	go w.loop(ctx)
}

func (w *Warmer) loop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		// Refresh pending count
		n, err := w.media.CountThumbPending()
		if err != nil {
			slog.Warn("thumb warmer: count pending", "err", err)
			sleep(ctx, 10*time.Second)
			continue
		}
		w.pending.Store(int64(n))

		if n == 0 {
			// Nothing to do — check again in 60s in case a scan added new items
			w.running.Store(false)
			sleep(ctx, 60*time.Second)
			continue
		}

		// Check if paused by a foreground request
		last := w.paused.Load()
		if last != 0 {
			elapsed := time.Since(time.Unix(0, last))
			if elapsed < warmerResumeWait {
				w.running.Store(false)
				sleep(ctx, warmerResumeWait-elapsed)
				continue
			}
		}

		// Process one item
		item, err := w.media.NextThumbPending()
		if err != nil || item == nil {
			sleep(ctx, 5*time.Second)
			continue
		}

		w.running.Store(true)

		absPath, err := util.SafeJoin(w.rootPath, item.RelativePath)
		if err != nil {
			slog.Warn("thumb warmer: unsafe path", "path", item.RelativePath)
			// Mark ready to avoid retrying a permanently bad path
			_ = w.media.SetThumbReady(item.ID, true)
			continue
		}

		switch item.Type {
		case "image":
			_, err = Generate(absPath, w.cacheDir, w.defaultWidth, w.defaultHeight)
		case "video":
			if FFmpegAvailable() {
				_, err = GenerateVideoPoster(absPath, w.cacheDir, w.defaultWidth, w.defaultHeight)
			} else {
				// No ffmpeg — skip permanently so we don't retry forever
				err = nil
			}
		}

		if err != nil {
			slog.Warn("thumb warmer: generation failed, skipping permanently", "path", item.RelativePath, "err", err)
		} else {
			slog.Debug("thumb warmer: generated", "path", item.RelativePath, "width", w.defaultWidth, "height", w.defaultHeight)
		}
		// Mark ready regardless of outcome — foreground handler generates on demand
		// and serves a placeholder on failure, so retrying a permanently broken item
		// (e.g. HEIC without ffmpeg) would just burn CPU forever.
		if setErr := w.media.SetThumbReady(item.ID, true); setErr != nil {
			slog.Warn("thumb warmer: set thumb_ready", "id", item.ID, "err", setErr)
		}

		sleep(ctx, warmerItemDelay)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
