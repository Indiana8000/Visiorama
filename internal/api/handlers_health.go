package api

import (
	"net/http"
	"os"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
)

var startTime = time.Now()

// warmerStatus is the subset of thumbs.Warmer used by the health handler.
type warmerStatus interface {
	Running() bool
	Pending() int64
}

type healthHandler struct {
	cfg    *app.Config
	store  *index.Store
	warmer warmerStatus
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

	ws := ThumbWarmerStatus{}
	if h.warmer != nil {
		ws.Running = h.warmer.Running()
		ws.PendingItems = h.warmer.Pending()
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status:             status,
		MediaRootAvailable: mediaRootOK,
		DatabaseAvailable:  dbOK,
		UptimeSeconds:      int64(time.Since(startTime).Seconds()),
		ThumbWarmer:        ws,
	})
}
