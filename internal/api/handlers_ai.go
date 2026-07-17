package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Indiana8000/visiorama/internal/ai"
	"github.com/Indiana8000/visiorama/internal/app"
)

type aiHandler struct {
	cfg    *app.Config
	client *ai.Client // nil when visiorama-ai is not available
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

	writeJSON(w, http.StatusOK, AIStatusResponse{
		Available:    true,
		Version:      sidecarStatus.Version,
		LoadedModels: sidecarStatus.LoadedModels,
		QueueDepth:   sidecarStatus.QueueDepth,
		Workers:      sidecarStatus.Workers,
	})
}
