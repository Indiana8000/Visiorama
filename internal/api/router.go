package api

import (
	"net/http"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/convert"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/scan"
	"github.com/Indiana8000/visiorama/internal/thumbs"
	"github.com/Indiana8000/visiorama/internal/transcode"
)

func NewRouter(cfg *app.Config, store *index.Store, warmer *thumbs.Warmer, tcRunner *transcode.Runner, imgCache *convert.Cache) http.Handler {
	mux := http.NewServeMux()
	runner := scan.NewRunner(cfg, store)
	runner.SetWarmer(warmer)

	ah := &albumsHandler{store: store}
	mux.HandleFunc("GET /api/albums/root", ah.getRoot)
	mux.HandleFunc("GET /api/albums/by-path", ah.getByPath)
	mux.HandleFunc("GET /api/albums/{albumId}", ah.getByID)

	thumbSem := make(chan struct{}, cfg.Scan.MaxWorkers)
	mh := &mediaHandler{cfg: cfg, store: store, warmer: warmer, thumbSem: thumbSem, runner: runner}
	mux.HandleFunc("GET /api/media/{mediaId}/metadata", mh.getMetadata)
	mux.HandleFunc("GET /api/media/{mediaId}/thumbnail", mh.getThumbnail)
	mux.HandleFunc("GET /api/media/{mediaId}/stream", mh.stream)

	sh := &scanHandler{cfg: cfg, store: store, runner: runner}
	mux.HandleFunc("POST /api/scans", sh.trigger)
	mux.HandleFunc("GET /api/scans", sh.getAll)
	mux.HandleFunc("GET /api/scans/active", sh.getActive)
	mux.HandleFunc("GET /api/scans/{scanId}", sh.getStatus)

	cvh := &convertHandler{cfg: cfg, store: store, cache: imgCache}
	mux.HandleFunc("GET /api/media/{mediaId}/convert", cvh.serve)

	tch := &transcodeHandler{store: store, runner: tcRunner}
	mux.HandleFunc("POST /api/media/{mediaId}/transcode", tch.trigger)
	mux.HandleFunc("GET /api/transcode-jobs/{jobId}", tch.getStatus)
	mux.HandleFunc("GET /api/media/{mediaId}/transcode/stream", tch.stream)

	hh := &healthHandler{cfg: cfg, store: store, warmer: warmer}
	mux.HandleFunc("GET /api/health", hh.health)

	adh := &adminHandler{cfg: cfg, store: store}
	mux.HandleFunc("GET /api/reset_thumbs", adh.resetThumbs)

	mh2 := &mapHandler{store: store}
	mux.HandleFunc("GET /api/map/clusters", mh2.getClusters)
	mux.HandleFunc("GET /api/map/style", mh2.getStyle)
	mux.HandleFunc("GET /api/albums/{albumId}/gps-count", mh2.getGPSCount)

	// SPA fallback — serves embedded Vue dist for all non-API paths
	mux.Handle("/", newSPAHandler())

	return mux
}
