package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/Indiana8000/visiorama/internal/ai"
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

	if os.Getenv("GOMEMLIMIT") == "" {
		const fallback = 3 * 1024 * 1024 * 1024
		var limit int64
		if cfg.Server.MemLimitMiB > 0 {
			limit = int64(cfg.Server.MemLimitMiB) * 1024 * 1024
			slog.Info("set GOMEMLIMIT from config", "limit", fmt.Sprintf("%d MiB", cfg.Server.MemLimitMiB))
		} else {
			limit = fallback
			if total := totalPhysicalBytes(); total > 0 {
				limit = int64(float64(total) * 0.9)
			}
			slog.Info("set GOMEMLIMIT", "limit", fmt.Sprintf("%d MiB", limit/(1024*1024)))
		}
		debug.SetMemoryLimit(limit)
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
	aiRepo := repositories.NewAIRepo(store.DB())
	if err := aiRepo.FailStale(now); err != nil {
		slog.Warn("failed to clean up stale ai jobs", "err", err)
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

	transcodeDir := cfg.Transcode.CacheDir
	if transcodeDir == "" {
		transcodeDir = filepath.Join(filepath.Dir(cfg.Thumbnails.CacheDir), "transcodes")
	}
	if err := os.MkdirAll(transcodeDir, 0755); err != nil {
		return fmt.Errorf("create transcode cache dir: %w", err)
	}

	// Resolve AI model/face-cache dirs so serveCrop can find files written by the sidecar.
	dbDir := filepath.Dir(cfg.Database.SQLitePath)
	if cfg.AI.ModelDir == "" {
		cfg.AI.ModelDir = filepath.Join(dbDir, "models")
	}
	if cfg.AI.FaceCacheDir == "" {
		cfg.AI.FaceCacheDir = filepath.Join(dbDir, "crops")
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

	// Detect optional visiorama-ai sidecar.
	// Detection order: (1) ping the socket — sidecar may be running without the binary in PATH,
	// e.g. started manually in dev or managed by an external supervisor.
	// (2) fall back to PATH lookup for informational logging only.
	socketPath := cfg.AI.SocketPath
	if socketPath == "" {
		socketPath = "/tmp/visiorama-ai.sock"
	}
	var aiClient *ai.Client
	pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
	probe := ai.NewClient(socketPath)
	if err := probe.Ping(pingCtx); err == nil {
		aiClient = probe
		slog.Info("visiorama-ai sidecar connected", "socket", socketPath)
	} else if ai.BinaryAvailable(cfg.AI.Binary) {
		// Binary exists but socket not yet ready — keep client so queue can retry.
		aiClient = probe
		slog.Info("visiorama-ai found but not yet reachable (will retry on first use)",
			"path", ai.BinaryPath(cfg.AI.Binary), "socket", socketPath, "err", err)
	} else {
		slog.Info("visiorama-ai not found — AI recognition features disabled",
			"socket", socketPath)
	}
	pingCancel()

	// AI queue — start only when sidecar is available or binary can be spawned.
	var aiQueue *ai.QueueRunner
	if aiClient != nil || ai.BinaryAvailable(cfg.AI.Binary) {
		aiQueue = ai.NewQueueRunner(cfg, aiRepo, aiClient)
		go aiQueue.Start(ctx)
		slog.Info("ai queue runner started")
	}

	handler := api.NewRouter(cfg, store, warmer, tcRunner, imgCache, aiClient, aiQueue)

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

	workers := runtime.NumCPU() +1
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
