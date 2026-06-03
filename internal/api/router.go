package api

import (
	"net/http"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/scan"
)

func NewRouter(cfg *app.Config, store *index.Store) http.Handler {
	mux := http.NewServeMux()
	runner := scan.NewRunner(cfg, store)

	ah := &albumsHandler{store: store}
	mux.HandleFunc("GET /api/albums/root", ah.getRoot)
	mux.HandleFunc("GET /api/albums/by-path", ah.getByPath)
	mux.HandleFunc("GET /api/albums/{albumId}", ah.getByID)

	mh := &mediaHandler{cfg: cfg, store: store}
	mux.HandleFunc("GET /api/media/{mediaId}/metadata", mh.getMetadata)
	mux.HandleFunc("GET /api/media/{mediaId}/thumbnail", mh.getThumbnail)
	mux.HandleFunc("GET /api/media/{mediaId}/stream", mh.stream)

	sh := &scanHandler{cfg: cfg, store: store, runner: runner}
	mux.HandleFunc("POST /api/scans", sh.trigger)
	mux.HandleFunc("GET /api/scans/{scanId}", sh.getStatus)

	hh := &healthHandler{cfg: cfg, store: store}
	mux.HandleFunc("GET /api/health", hh.health)

	// SPA fallback — serves embedded Vue dist for all non-API paths
	mux.Handle("/", newSPAHandler())

	return mux
}
