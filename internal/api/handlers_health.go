package api

import (
	"net/http"
	"os"
	"time"

	"github.com/USERNAME/visiorama/internal/app"
	"github.com/USERNAME/visiorama/internal/index"
)

var startTime = time.Now()

type healthHandler struct {
	cfg   *app.Config
	store *index.Store
}

func (h *healthHandler) health(w http.ResponseWriter, r *http.Request) {
	mediaRootOK := false
	if _, err := os.Stat(h.cfg.Library.RootPath); err == nil {
		mediaRootOK = true
	}

	dbOK := h.store.Ping() == nil

	status := "ok"
	if !mediaRootOK || !dbOK {
		status = "degraded"
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status:             status,
		MediaRootAvailable: mediaRootOK,
		DatabaseAvailable:  dbOK,
		UptimeSeconds:      int64(time.Since(startTime).Seconds()),
	})
}
