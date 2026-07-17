package transcode

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/util"
)

type Runner struct {
	mu    sync.Mutex
	queue chan string // job IDs
	cfg   *app.Config
	store *index.Store
}

func NewRunner(cfg *app.Config, store *index.Store) *Runner {
	return &Runner{
		cfg:   cfg,
		store: store,
		queue: make(chan string, 64),
	}
}

// Start launches the worker goroutine and the cleanup ticker.
func (r *Runner) Start(ctx context.Context) {
	go r.worker(ctx)
	go r.cleanupLoop(ctx)
}

// Enqueue creates a transcode job for the given media and queues it.
// Returns the job ID and a bool indicating if it was newly created (false = already exists/running).
func (r *Runner) Enqueue(mediaID int64) (string, bool, error) {
	repo := repositories.NewTranscodeRepo(r.store.DB())

	// Return existing active job if present
	existing, err := repo.GetLatestForMedia(mediaID)
	if err != nil {
		return "", false, err
	}
	if existing != nil && (existing.Status == "queued" || existing.Status == "running") {
		return existing.ID, false, nil
	}

	jobID := fmt.Sprintf("tc-%d-%d", mediaID, time.Now().UnixMilli())
	job := &repositories.TranscodeJob{
		ID:        jobID,
		MediaID:   mediaID,
		Status:    "queued",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := repo.Create(job); err != nil {
		return "", false, err
	}

	select {
	case r.queue <- jobID:
	default:
		slog.Warn("transcode queue full, job will be picked up on next restart", "jobID", jobID)
	}

	return jobID, true, nil
}

func (r *Runner) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case jobID := <-r.queue:
			r.process(jobID)
		}
	}
}

func (r *Runner) process(jobID string) {
	repo := repositories.NewTranscodeRepo(r.store.DB())
	mediaRepo := repositories.NewMediaRepo(r.store.DB())

	job, err := repo.GetByID(jobID)
	if err != nil || job == nil {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := repo.UpdateStatus(jobID, "running", nil, nil, nil); err != nil {
		slog.Warn("transcode: failed to persist running status", "jobID", jobID, "err", err)
	}

	m, err := mediaRepo.GetByID(job.MediaID)
	if err != nil || m == nil {
		errMsg := "media not found"
		if err := repo.UpdateStatus(jobID, "failed", nil, &errMsg, &now); err != nil {
			slog.Warn("transcode: failed to persist failed status", "jobID", jobID, "err", err)
		}
		return
	}

	srcPath, err := util.SafeJoin(r.cfg.Library.RootPath, m.RelativePath)
	if err != nil {
		errMsg := "invalid media path"
		if err := repo.UpdateStatus(jobID, "failed", nil, &errMsg, &now); err != nil {
			slog.Warn("transcode: failed to persist failed status", "jobID", jobID, "err", err)
		}
		return
	}

	cacheDir := r.cfg.Transcode.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(filepath.Dir(r.cfg.Thumbnails.CacheDir), "transcodes")
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		errMsg := fmt.Sprintf("mkdir: %v", err)
		if err := repo.UpdateStatus(jobID, "failed", nil, &errMsg, &now); err != nil {
			slog.Warn("transcode: failed to persist failed status", "jobID", jobID, "err", err)
		}
		return
	}

	outPath := filepath.Join(cacheDir, fmt.Sprintf("%d.mp4", job.MediaID))

	args := []string{
		"-y",
		"-hwaccel", "auto",
		"-i", srcPath,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-c:a", "aac",
		"-movflags", "+faststart",
		outPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	finAt := time.Now().UTC().Format(time.RFC3339)

	if err != nil {
		errMsg := fmt.Sprintf("ffmpeg: %v — %s", err, string(out))
		slog.Warn("transcode failed", "jobID", jobID, "err", errMsg)
		if err := repo.UpdateStatus(jobID, "failed", nil, &errMsg, &finAt); err != nil {
			slog.Warn("transcode: failed to persist failed status", "jobID", jobID, "err", err)
		}
		return
	}

	if err := repo.UpdateStatus(jobID, "success", &outPath, nil, &finAt); err != nil {
		slog.Warn("transcode: failed to persist success status", "jobID", jobID, "err", err)
	}
	slog.Info("transcode complete", "jobID", jobID, "out", outPath)
}

func (r *Runner) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	r.runCleanup()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runCleanup()
		}
	}
}

func (r *Runner) runCleanup() {
	ttl := r.cfg.Transcode.TTLHours
	if ttl <= 0 {
		ttl = 48
	}
	threshold := time.Now().UTC().Add(-time.Duration(ttl) * time.Hour).Format(time.RFC3339)
	repo := repositories.NewTranscodeRepo(r.store.DB())
	paths, err := repo.DeleteExpired(threshold)
	if err != nil {
		slog.Warn("transcode cleanup query failed", "err", err)
		return
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			slog.Warn("transcode cleanup: remove failed", "path", p, "err", err)
		}
	}
	if len(paths) > 0 {
		slog.Info("transcode cleanup", "removed", len(paths))
	}
}
