package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/Indiana8000/visiorama/internal/api"
	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/convert"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/observability"
	"github.com/Indiana8000/visiorama/internal/thumbs"
	"github.com/Indiana8000/visiorama/internal/transcode"
	"github.com/Indiana8000/visiorama/internal/util"
)

func Run(cfg *app.Config) error {
	observability.SetupLogging()
	util.RegisterMIMETypes()

	// Set GOMEMLIMIT to 90% of physical RAM so GC collects aggressively
	// before the OS starts paging. Falls back to 3 GiB if RAM can't be read.
	// Override with GOMEMLIMIT env var (in bytes) if needed.
	if os.Getenv("GOMEMLIMIT") == "" {
		const fallback = 3 * 1024 * 1024 * 1024
		limit := int64(fallback)
		if total := totalPhysicalBytes(); total > 0 {
			limit = int64(float64(total) * 0.9)
		}
		debug.SetMemoryLimit(limit)
		slog.Info("set GOMEMLIMIT", "limit", fmt.Sprintf("%d MiB", limit/(1024*1024)))
	}

	cfg.Scan.MaxWorkers = resolveWorkers(cfg.Scan.MaxWorkers)
	slog.Info("scan workers", "count", cfg.Scan.MaxWorkers)

	store, err := index.Open(cfg.Database.SQLitePath)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer store.Close()

	if err := index.Migrate(store); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Mark any jobs left in queued/running state by a previous crash as failed.
	now := time.Now().UTC().Format(time.RFC3339)
	scanRepo := repositories.NewScanRepo(store.DB())
	if err := scanRepo.FailStale(now); err != nil {
		slog.Warn("failed to clean up stale scan jobs", "err", err)
	}
	tcRepo := repositories.NewTranscodeRepo(store.DB())
	if err := tcRepo.FailStale(now); err != nil {
		slog.Warn("failed to clean up stale transcode jobs", "err", err)
	}

	defaultWidth := 320
	if len(cfg.Thumbnails.Sizes) > 0 {
		defaultWidth = cfg.Thumbnails.Sizes[0]
	}
	defaultHeight := cfg.Thumbnails.ThumbHeight(defaultWidth)
	mediaRepo := repositories.NewMediaRepo(store.DB())
	warmer := thumbs.NewWarmer(mediaRepo, cfg.Library.RootPath, cfg.Thumbnails.CacheDir, defaultWidth, defaultHeight)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	warmer.Start(ctx)

	tcRunner := transcode.NewRunner(cfg, store)
	tcRunner.Start(ctx)

	imgCache := convert.NewCache()

	if err := os.MkdirAll(cfg.Thumbnails.CacheDir, 0755); err != nil {
		return fmt.Errorf("create thumbnail cache dir: %w", err)
	}

	if thumbs.ImageMagickAvailable() {
		slog.Info("imagemagick found", "path", thumbs.ImageMagickPath())
	} else {
		slog.Warn("imagemagick not found — magick not in PATH", "PATH", os.Getenv("PATH"))
	}
	if thumbs.FFmpegAvailable() {
		slog.Info("ffmpeg found", "path", thumbs.FFmpegPath())
	} else {
		slog.Warn("ffmpeg not found — video thumbnails unavailable; install ffmpeg or add it to PATH")
	}

	handler := api.NewRouter(cfg, store, warmer, tcRunner, imgCache)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // streaming endpoints require no write deadline
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutCtx)
}

// resolveWorkers computes the effective worker count:
//   - auto = min(numCPU, totalRAM/512MiB); RAM unknown → numCPU only
//   - maxCfg > 0 caps the result; maxCfg == 0 means no cap (pure auto)
//   - floor is 1
func resolveWorkers(maxCfg int) int {
	const ramPerWorker = 512 * 1024 * 1024

	workers := runtime.NumCPU()
	if total := totalPhysicalBytes(); total > 0 {
		byRAM := int(total / ramPerWorker)
		if byRAM < workers {
			workers = byRAM
		}
	}
	if maxCfg > 0 && maxCfg < workers {
		workers = maxCfg
	}
	if workers < 1 {
		workers = 1
	}
	return workers
}
