package api

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/Indiana8000/visiorama/internal/ai"
	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/convert"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/scan"
	"github.com/Indiana8000/visiorama/internal/thumbs"
	"github.com/Indiana8000/visiorama/internal/transcode"
)

func NewRouter(cfg *app.Config, store *index.Store, warmer *thumbs.Warmer, tcRunner *transcode.Runner, imgCache *convert.Cache, aiClient *ai.Client, aiQueue *ai.QueueRunner, thumbSem chan struct{}, version string) http.Handler {
	mux := http.NewServeMux()
	runner := scan.NewRunner(cfg, store)
	runner.SetWarmer(warmer)
	if aiQueue != nil {
		runner.SetAIQueue(aiQueue)
	}

	ah := &albumsHandler{cfg: cfg, store: store, runner: runner}
	mux.HandleFunc("GET /api/albums/root", ah.getRoot)
	mux.HandleFunc("GET /api/albums/by-path", ah.getByPath)
	mux.HandleFunc("POST /api/albums/by-media-ids", ah.albumsByMediaIDs)
	mux.HandleFunc("GET /api/albums/{albumId}", ah.getByID)

	mh := &mediaHandler{cfg: cfg, store: store, warmer: warmer, thumbSem: thumbSem, runner: runner}
	mux.HandleFunc("GET /api/media/{mediaId}/metadata", mh.getMetadata)
	mux.HandleFunc("GET /api/media/{mediaId}/thumbnail", mh.getThumbnail)
	mux.HandleFunc("GET /api/media/{mediaId}/stream", mh.stream)
	mux.HandleFunc("GET /api/media/{mediaId}/ai", mh.getAI)

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

	hh := &healthHandler{cfg: cfg, store: store, warmer: warmer, version: version}
	mux.HandleFunc("GET /api/health", hh.health)

	adh := &adminHandler{cfg: cfg, store: store}
	mux.HandleFunc("DELETE /api/reset_thumbs", adh.resetThumbs)

	aih := &aiHandler{cfg: cfg, store: store, client: aiClient, queue: aiQueue}
	mux.HandleFunc("GET /api/ai/status", aih.status)
	mux.HandleFunc("POST /api/ai/reanalyze", aih.reanalyze)
	mux.HandleFunc("POST /api/ai/cleanup", aih.cleanup)

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
	mux.HandleFunc("DELETE /api/ai/faces/{faceId}/person", ph.unassignFace)

	mh2 := &mapHandler{store: store}
	mux.HandleFunc("GET /api/map/clusters", mh2.getClusters)
	mux.HandleFunc("GET /api/map/style", mh2.getStyle)
	mux.HandleFunc("GET /api/map/proxy/{path...}", mh2.proxyUpstream)
	mux.HandleFunc("GET /api/albums/{albumId}/gps-count", mh2.getGPSCount)
	mux.HandleFunc("GET /api/gps-count", mh2.getGPSCountGlobal)

	// SPA fallback — serves embedded Vue dist for all non-API paths
	mux.Handle("/", newSPAHandler())

	return recoverMiddleware(mux)
}

// recoverMiddleware stops a handler panic from crashing the whole process.
// A native crash inside the AI sidecar can't be caught this way (it's a
// separate OS process), but a bug in a Go handler no longer takes the server down.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "err", rec, "path", r.URL.Path, "stack", string(debug.Stack()))
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
