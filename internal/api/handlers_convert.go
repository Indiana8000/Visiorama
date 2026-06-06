package api

import (
	"bytes"
	"net/http"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/convert"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/util"
)

type convertHandler struct {
	cfg   *app.Config
	store *index.Store
	cache *convert.Cache
}

func (h *convertHandler) serve(w http.ResponseWriter, r *http.Request) {
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
	if m.Type != "image" {
		badRequest(w, "not an image")
		return
	}

	absPath, err := util.SafeJoin(h.cfg.Library.RootPath, m.RelativePath)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	maxDim := h.cfg.Transcode.ImageMaxDim
	if maxDim <= 0 {
		maxDim = 2400
	}

	data, err := h.cache.Do(id, 5*time.Minute, func() ([]byte, error) {
		return convert.ToJPEG(absPath, maxDim)
	})
	if err != nil {
		http.Error(w, "conversion failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, "converted.jpg", time.Time{}, bytes.NewReader(data))
}
