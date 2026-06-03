package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/USERNAME/visiorama/internal/app"
	"github.com/USERNAME/visiorama/internal/index"
	"github.com/USERNAME/visiorama/internal/index/repositories"
	"github.com/USERNAME/visiorama/internal/thumbs"
	"github.com/USERNAME/visiorama/internal/util"
)

type mediaHandler struct {
	cfg   *app.Config
	store *index.Store
}

func (h *mediaHandler) getMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMediaID(w, r)
	if !ok {
		return
	}
	mediaRepo := repositories.NewMediaRepo(h.store.DB())
	m, err := mediaRepo.GetByID(id)
	if err != nil || m == nil {
		notFound(w)
		return
	}
	warningLarge := m.SizeBytes >= h.cfg.Limits.LargeMediaWarningBytes
	writeJSON(w, http.StatusOK, repoMediaToMetadata(m, warningLarge))
}

func (h *mediaHandler) getThumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMediaID(w, r)
	if !ok {
		return
	}
	size := 240
	if v := r.URL.Query().Get("size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || (n != 240 && n != 480 && n != 960) {
			badRequest(w, "size must be one of 240, 480, 960")
			return
		}
		size = n
	}

	mediaRepo := repositories.NewMediaRepo(h.store.DB())
	m, err := mediaRepo.GetByID(id)
	if err != nil || m == nil {
		notFound(w)
		return
	}

	absPath, err := util.SafeJoin(h.cfg.Library.RootPath, m.RelativePath)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var cachePath string
	switch m.Type {
	case "image":
		cachePath, err = thumbs.Generate(absPath, h.cfg.Thumbnails.CacheDir, size)
	case "video":
		if !thumbs.FFmpegAvailable() {
			servePlaceholder(w)
			return
		}
		cachePath, err = thumbs.GenerateVideoPoster(absPath, h.cfg.Thumbnails.CacheDir, size)
	default:
		servePlaceholder(w)
		return
	}
	if err != nil {
		slog.Warn("thumbnail generation failed, serving placeholder", "id", id, "err", err)
		servePlaceholder(w)
		return
	}

	f, err := os.Open(cachePath)
	if err != nil {
		notFound(w)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "image/jpeg")
	info, _ := f.Stat()
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func (h *mediaHandler) stream(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMediaID(w, r)
	if !ok {
		return
	}
	mediaRepo := repositories.NewMediaRepo(h.store.DB())
	m, err := mediaRepo.GetByID(id)
	if err != nil || m == nil {
		notFound(w)
		return
	}

	absPath, err := util.SafeJoin(h.cfg.Library.RootPath, m.RelativePath)
	if err != nil {
		slog.Warn("path traversal attempt", "relPath", m.RelativePath)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		notFound(w)
		return
	}
	defer f.Close()

	ct := m.MimeType
	if ct == "" {
		ct = util.TypeByExtension(fmt.Sprintf(".%s", m.Extension))
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)

	var modTime time.Time
	if info, err := f.Stat(); err == nil {
		modTime = info.ModTime()
	}
	http.ServeContent(w, r, m.Filename, modTime, f)
}

func servePlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(thumbs.PlaceholderSVG)
}

func parseMediaID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := r.PathValue("mediaId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		badRequest(w, "mediaId must be an integer")
		return 0, false
	}
	return id, true
}
