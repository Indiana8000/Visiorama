package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Indiana8000/visiorama/internal/ai"
	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

type aiHandler struct {
	cfg    *app.Config
	store  *index.Store
	client *ai.Client      // nil when visiorama-ai is not available
	queue  *ai.QueueRunner // nil when AI not configured
}

// status returns the AI sidecar availability and, if reachable, its current state.
func (h *aiHandler) status(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		writeJSON(w, http.StatusOK, AIStatusResponse{
			Available: false,
			Reason:    "visiorama-ai binary not found",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	sidecarStatus, err := h.client.Status(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, AIStatusResponse{
			Available: false,
			Reason:    "sidecar unreachable: " + err.Error(),
		})
		return
	}

	resp := AIStatusResponse{
		Available:    true,
		Version:      sidecarStatus.Version,
		LoadedModels: sidecarStatus.LoadedModels,
		QueueDepth:   sidecarStatus.QueueDepth,
		Workers:      sidecarStatus.Workers,
	}
	if h.queue != nil {
		s := h.queue.Stats()
		resp.JobsQueued = int(s.Queued.Load())
		resp.JobsRunning = int(s.Running.Load())
		resp.JobsDone = int(s.Done.Load())
		resp.JobsFailed = int(s.Failed.Load())
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /api/ai/reanalyze?albumPath=... — re-queue all media in the given album for AI analysis.
func (h *aiHandler) reanalyze(w http.ResponseWriter, r *http.Request) {
	if h.queue == nil {
		writeError(w, http.StatusServiceUnavailable, "ai_unavailable", "AI queue not running")
		return
	}
	albumPath := r.URL.Query().Get("albumPath")
	now := time.Now().UTC().Format(time.RFC3339)
	aiRepo := repositories.NewAIRepo(h.store.DB())
	if err := aiRepo.EnqueueForAlbum(albumPath, now); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	slog.Info("ai: reanalyze triggered", "albumPath", albumPath)
	writeJSON(w, http.StatusOK, map[string]any{"queued": true, "albumPath": albumPath})
}

// POST /api/ai/cleanup — delete AI data for media no longer in the library.
func (h *aiHandler) cleanup(w http.ResponseWriter, r *http.Request) {
	aiRepo := repositories.NewAIRepo(h.store.DB())
	n, err := aiRepo.DeleteOrphanedAIData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	slog.Info("ai: orphaned data cleaned up", "rows", n)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}
