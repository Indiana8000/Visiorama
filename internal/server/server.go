package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/Indiana8000/visiorama/internal/api"
	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/observability"
	"github.com/Indiana8000/visiorama/internal/thumbs"
	"github.com/Indiana8000/visiorama/internal/util"
)

func Run(cfg *app.Config) error {
	observability.SetupLogging()
	util.RegisterMIMETypes()

	// Keep Go GC aggressive so image buffers are freed promptly.
	// 3 GiB soft limit — GC collects harder before reaching this threshold.
	// Override with GOMEMLIMIT env var (in bytes) if needed.
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(3 * 1024 * 1024 * 1024)
	}

	store, err := index.Open(cfg.Database.SQLitePath)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer store.Close()

	if err := index.Migrate(store); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Mark any jobs left in queued/running state by a previous crash as failed.
	scanRepo := repositories.NewScanRepo(store.DB())
	if err := scanRepo.FailStale(time.Now().UTC().Format(time.RFC3339)); err != nil {
		slog.Warn("failed to clean up stale scan jobs", "err", err)
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

	if err := os.MkdirAll(cfg.Thumbnails.CacheDir, 0755); err != nil {
		return fmt.Errorf("create thumbnail cache dir: %w", err)
	}

	if thumbs.FFmpegAvailable() {
		slog.Info("ffmpeg found", "path", thumbs.FFmpegPath())
	} else {
		slog.Warn("ffmpeg not found — HEIC/AVIF/video thumbnails unavailable; install ffmpeg or add it to PATH")
	}

	handler := api.NewRouter(cfg, store, warmer)

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
