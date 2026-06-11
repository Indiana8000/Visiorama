package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/thumbs"
	"github.com/Indiana8000/visiorama/internal/util"
)

func (h *mediaHandler) maybeStartOrphanScan() {
	if h.runner == nil || h.runner.IsRunning() {
		return
	}
	scanID := fmt.Sprintf("scan-orphan-%d", time.Now().UnixMilli())
	scanRepo := repositories.NewScanRepo(h.store.DB())
	_ = scanRepo.Create(&repositories.ScanJob{ID: scanID, Mode: "orphan", Status: "queued"})
	if err := h.runner.TriggerAsync(scanID, "orphan"); err != nil {
		slog.Warn("media: could not start orphan scan", "err", err)
	} else {
		slog.Info("media: triggered orphan scan", "scanId", scanID)
	}
}

type mediaHandler struct {
	cfg      *app.Config
	store    *index.Store
	warmer   thumbWarmer
	thumbSem chan struct{}
	runner   orphanTrigger
}

// orphanTrigger is the subset of scan.Runner used by the media handler.
type orphanTrigger interface {
	IsRunning() bool
	TriggerAsync(scanID, mode string) error
}

// thumbWarmer is the subset of thumbs.Warmer used by the media handler.
type thumbWarmer interface {
	Pause()
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
	validSizes := h.cfg.Thumbnails.Sizes
	width := validSizes[0]
	if v := r.URL.Query().Get("size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			badRequest(w, "size must be a number")
			return
		}
		valid := false
		for _, s := range validSizes {
			if n == s {
				valid = true
				break
			}
		}
		if !valid {
			badRequest(w, fmt.Sprintf("size must be one of %v", validSizes))
			return
		}
		width = n
	}
	height := h.cfg.Thumbnails.ThumbHeight(width)

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

	mediaRepo2 := repositories.NewMediaRepo(h.store.DB())
	thumbReady, _ := mediaRepo2.GetThumbReady(id)

	if !thumbReady {
		// Acquire semaphore slot — limits concurrent foreground thumbnail generation
		// across all clients to the same worker count used by the scanner.
		if h.thumbSem != nil {
			h.thumbSem <- struct{}{}
			defer func() { <-h.thumbSem }()
		}
		// Pause the background warmer while a foreground slot is held.
		if h.warmer != nil {
			h.warmer.Pause()
		}
	}

	if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
		h.maybeStartOrphanScan()
		notFound(w)
		return
	}

	var cachePath string
	switch m.Type {
	case "image":
		cachePath, err = thumbs.Generate(absPath, h.cfg.Thumbnails.CacheDir, width, height)
	case "video":
		if !thumbs.FFmpegAvailable() {
			servePlaceholder(w)
			return
		}
		cachePath, err = thumbs.GenerateVideoPoster(absPath, h.cfg.Thumbnails.CacheDir, width, height)
	default:
		servePlaceholder(w)
		return
	}
	if err != nil {
		slog.Warn("thumbnail generation failed, serving placeholder", "id", id, "err", err)
		servePlaceholder(w)
		return
	}

	// Mark ready in DB so the warmer skips this item
	if !thumbReady {
		_ = mediaRepo2.SetThumbReady(id, true)
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
		if os.IsNotExist(err) {
			h.maybeStartOrphanScan()
		}
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
