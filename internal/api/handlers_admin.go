package api

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

type adminHandler struct {
	cfg   *app.Config
	store *index.Store
}

func (h *adminHandler) resetThumbs(w http.ResponseWriter, r *http.Request) {
	// Delete all cached thumbnail files
	if err := os.RemoveAll(h.cfg.Thumbnails.CacheDir); err != nil {
		slog.Error("reset_thumbs: remove cache dir", "err", err)
		http.Error(w, "failed to delete thumbnail cache", http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(h.cfg.Thumbnails.CacheDir, 0755); err != nil {
		slog.Error("reset_thumbs: recreate cache dir", "err", err)
		http.Error(w, "failed to recreate thumbnail cache dir", http.StatusInternalServerError)
		return
	}

	// Reset all thumb_ready flags in DB
	mediaRepo := repositories.NewMediaRepo(h.store.DB())
	affected, err := mediaRepo.ResetAllThumbReady()
	if err != nil {
		slog.Error("reset_thumbs: reset db flags", "err", err)
		http.Error(w, "failed to reset thumb_ready flags", http.StatusInternalServerError)
		return
	}

	slog.Info("reset_thumbs: completed", "media_reset", affected)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"mediaReset":  affected,
	})
}
