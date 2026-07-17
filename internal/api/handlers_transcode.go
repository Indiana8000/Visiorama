package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/transcode"
)

type transcodeHandler struct {
	cfg    *app.Config
	store  *index.Store
	runner *transcode.Runner
}

func (h *transcodeHandler) trigger(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMediaID(w, r)
	if !ok {
		return
	}

	jobID, created, err := h.runner.Enqueue(id)
	if err != nil {
		http.Error(w, "failed to enqueue transcode job", http.StatusInternalServerError)
		return
	}

	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]string{"jobId": jobID})
}

func (h *transcodeHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if jobID == "" {
		badRequest(w, "jobId required")
		return
	}

	repo := repositories.NewTranscodeRepo(h.store.DB())
	job, err := repo.GetByID(jobID)
	if err != nil || job == nil {
		notFound(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jobId":      job.ID,
		"mediaId":    job.MediaID,
		"status":     job.Status,
		"createdAt":  job.CreatedAt,
		"finishedAt": job.FinishedAt,
		"error":      job.Error,
	})
}

func (h *transcodeHandler) stream(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMediaID(w, r)
	if !ok {
		return
	}

	repo := repositories.NewTranscodeRepo(h.store.DB())
	job, err := repo.GetLatestForMedia(id)
	if err != nil || job == nil || job.Status != "success" || job.OutputPath == nil {
		notFound(w)
		return
	}

	// Verify OutputPath is under the configured transcode cache dir to prevent
	// path traversal if a corrupted DB row contains an arbitrary path.
	outputPath := filepath.Clean(*job.OutputPath)
	cacheDir := filepath.Clean(h.cfg.Transcode.CacheDir)
	if !strings.HasPrefix(outputPath, cacheDir+string(filepath.Separator)) {
		notFound(w)
		return
	}

	f, err := os.Open(outputPath)
	if err != nil {
		notFound(w)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "video/mp4")
	var modTime time.Time
	if info, err := f.Stat(); err == nil {
		modTime = info.ModTime()
	}
	http.ServeContent(w, r, "transcode.mp4", modTime, f)
}
