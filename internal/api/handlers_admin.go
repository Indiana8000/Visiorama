package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

type adminHandler struct {
	cfg   *app.Config
	store *index.Store
}

// safeCacheDir returns an error if dir is empty, relative, or suspiciously short (e.g. "/" or "C:\").
func safeCacheDir(dir string) error {
	abs := filepath.Clean(dir)
	if !filepath.IsAbs(abs) {
		return fmt.Errorf("thumbnails.cacheDir must be absolute, got %q", dir)
	}
	// Reject root or single-segment paths like "/tmp" on Linux or "C:\" on Windows.
	parent := filepath.Dir(abs)
	if parent == abs || strings.Count(filepath.ToSlash(abs), "/") < 2 {
		return fmt.Errorf("thumbnails.cacheDir %q is too close to filesystem root", dir)
	}
	return nil
}

func (h *adminHandler) resetThumbs(w http.ResponseWriter, r *http.Request) {
	if err := safeCacheDir(h.cfg.Thumbnails.CacheDir); err != nil {
		slog.Error("reset_thumbs: unsafe cacheDir", "err", err)
		http.Error(w, "server misconfiguration: unsafe cache directory", http.StatusInternalServerError)
		return
	}
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
