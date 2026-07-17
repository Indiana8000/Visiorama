package api

import (
	"net/http"

	"github.com/Indiana8000/visiorama/internal/ai"
	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/convert"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/scan"
	"github.com/Indiana8000/visiorama/internal/thumbs"
	"github.com/Indiana8000/visiorama/internal/transcode"
)

func NewRouter(cfg *app.Config, store *index.Store, warmer *thumbs.Warmer, tcRunner *transcode.Runner, imgCache *convert.Cache, aiClient *ai.Client, aiQueue *ai.QueueRunner) http.Handler {
	mux := http.NewServeMux()
	runner := scan.NewRunner(cfg, store)
	runner.SetWarmer(warmer)
	if aiQueue != nil {
		runner.SetAIQueue(aiQueue)
	}

	ah := &albumsHandler{store: store}
	mux.HandleFunc("GET /api/albums/root", ah.getRoot)
	mux.HandleFunc("GET /api/albums/by-path", ah.getByPath)
	mux.HandleFunc("POST /api/albums/by-media-ids", ah.albumsByMediaIDs)
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

	tch := &transcodeHandler{cfg: cfg, store: store, runner: tcRunner}
	mux.HandleFunc("POST /api/media/{mediaId}/transcode", tch.trigger)
	mux.HandleFunc("GET /api/transcode-jobs/{jobId}", tch.getStatus)
	mux.HandleFunc("GET /api/media/{mediaId}/transcode/stream", tch.stream)

	hh := &healthHandler{cfg: cfg, store: store, warmer: warmer}
	mux.HandleFunc("GET /api/health", hh.health)

	adh := &adminHandler{cfg: cfg, store: store}
	mux.HandleFunc("GET /api/reset_thumbs", adh.resetThumbs)

	aih := &aiHandler{cfg: cfg, client: aiClient, queue: aiQueue}
	mux.HandleFunc("GET /api/ai/status", aih.status)

	ph := &personsHandler{cfg: cfg, store: store}
	mux.HandleFunc("GET /api/ai/clusters", ph.getClusters)
	mux.HandleFunc("DELETE /api/ai/clusters/{clusterId}/faces/{faceId}", ph.removeFaceFromCluster)
	mux.HandleFunc("GET /api/ai/persons", ph.listPersons)
	mux.HandleFunc("POST /api/ai/persons", ph.createPerson)
	mux.HandleFunc("PUT /api/ai/persons/{personId}", ph.renamePerson)
	mux.HandleFunc("DELETE /api/ai/persons/{personId}", ph.deletePerson)
	mux.HandleFunc("POST /api/ai/persons/{personId}/merge/{otherId}", ph.mergePersons)
	mux.HandleFunc("GET /api/ai/persons/{personId}/media", ph.getPersonMedia)
	mux.HandleFunc("GET /api/ai/counts", ph.statusCounts)
	mux.HandleFunc("GET /api/ai/crops/{filename}", ph.serveCrop)

	mh2 := &mapHandler{store: store}
	mux.HandleFunc("GET /api/map/clusters", mh2.getClusters)
	mux.HandleFunc("GET /api/map/style", mh2.getStyle)
	mux.HandleFunc("GET /api/map/proxy/{path...}", mh2.proxyUpstream)
	mux.HandleFunc("GET /api/albums/{albumId}/gps-count", mh2.getGPSCount)

	// SPA fallback — serves embedded Vue dist for all non-API paths
	mux.Handle("/", newSPAHandler())

	return mux
}
