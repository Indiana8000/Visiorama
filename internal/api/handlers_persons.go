package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Indiana8000/visiorama/internal/ai"
	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

const (
	dbscanEps    = 0.35 // cosine-distance threshold (ArcFace L2-norm: same person <0.3, different >0.4)
	dbscanMinPts = 1
	defaultPageSize = 50
)

type personsHandler struct {
	cfg   *app.Config
	store *index.Store
}

// GET /api/ai/clusters — run DBSCAN on unassigned faces, return unreviewed clusters
func (h *personsHandler) getClusters(w http.ResponseWriter, r *http.Request) {
	repo := repositories.NewPersonsRepo(h.store.DB())

	// Re-cluster unassigned faces on each request (cheap for typical library sizes).
	faces, err := repo.UnassignedFaceEmbeddings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	if len(faces) > 0 {
		feSlice := make([]ai.FaceEmbedding, len(faces))
		for i, f := range faces {
			feSlice[i] = ai.FaceEmbedding{ID: f.ID, Embedding: f.Embedding}
		}
		assignments := ai.ClusterFaces(feSlice, dbscanEps, dbscanMinPts)
		if err := repo.SaveClusterAssignments(assignments); err != nil {
			slog.Warn("persons: save cluster assignments failed", "err", err)
		}
	}

	clusters, err := repo.ListClusters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}

	out := make([]ClusterDTO, 0, len(clusters))
	for _, c := range clusters {
		faces := make([]ClusterFaceDTO, 0, len(c.Faces))
		for _, f := range c.Faces {
			faces = append(faces, ClusterFaceDTO{
				FaceID:   f.FaceID,
				MediaID:  f.MediaID,
				CropPath: h.cropURL(f.CropPath),
				BBoxJSON: f.BBoxJSON,
			})
		}
		out = append(out, ClusterDTO{ClusterID: c.ClusterID, Faces: faces})
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/ai/persons — body: {clusterId, name}
func (h *personsHandler) createPerson(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClusterID int64  `json:"clusterId"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name required")
		return
	}
	repo := repositories.NewPersonsRepo(h.store.DB())
	if err := repo.NameCluster(body.ClusterID, body.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"personId": body.ClusterID, "name": body.Name})
}

// PUT /api/ai/persons/{personId} — body: {name}
func (h *personsHandler) renamePerson(w http.ResponseWriter, r *http.Request) {
	personID, ok := parseID(w, r, "personId")
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name required")
		return
	}
	repo := repositories.NewPersonsRepo(h.store.DB())
	if err := repo.NameCluster(personID, body.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"personId": personID, "name": body.Name})
}

// DELETE /api/ai/persons/{personId}
func (h *personsHandler) deletePerson(w http.ResponseWriter, r *http.Request) {
	personID, ok := parseID(w, r, "personId")
	if !ok {
		return
	}
	repo := repositories.NewPersonsRepo(h.store.DB())
	if err := repo.DeletePerson(personID); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/ai/persons/{personId}/merge/{otherId}
func (h *personsHandler) mergePersons(w http.ResponseWriter, r *http.Request) {
	dstID, ok := parseID(w, r, "personId")
	if !ok {
		return
	}
	srcID, ok := parseID(w, r, "otherId")
	if !ok {
		return
	}
	if dstID == srcID {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot merge person with itself")
		return
	}
	repo := repositories.NewPersonsRepo(h.store.DB())
	if err := repo.MergePersons(dstID, srcID); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"personId": dstID})
}

// DELETE /api/ai/clusters/{clusterId}/faces/{faceId}
func (h *personsHandler) removeFaceFromCluster(w http.ResponseWriter, r *http.Request) {
	faceID, ok := parseID(w, r, "faceId")
	if !ok {
		return
	}
	repo := repositories.NewPersonsRepo(h.store.DB())
	if err := repo.RemoveFaceFromCluster(faceID); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/ai/persons — list named persons
func (h *personsHandler) listPersons(w http.ResponseWriter, r *http.Request) {
	repo := repositories.NewPersonsRepo(h.store.DB())
	persons, err := repo.ListPersons()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	out := make([]PersonDTO, 0, len(persons))
	for _, p := range persons {
		var crop *string
		if p.CoverCrop != nil {
			u := h.cropURL(*p.CoverCrop)
			crop = &u
		}
		out = append(out, PersonDTO{
			ID:         p.ID,
			Name:       p.Name,
			CoverCrop:  crop,
			FaceCount:  p.FaceCount,
			MediaCount: p.MediaCount,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/ai/persons/{personId}/media?page=1&pageSize=50
func (h *personsHandler) getPersonMedia(w http.ResponseWriter, r *http.Request) {
	personID, ok := parseID(w, r, "personId")
	if !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 || pageSize > 200 {
		pageSize = defaultPageSize
	}

	pRepo := repositories.NewPersonsRepo(h.store.DB())
	mediaIDs, total, err := pRepo.GetPersonMedia(personID, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}

	mRepo := repositories.NewMediaRepo(h.store.DB())
	mediaRows, err := mRepo.GetByIDs(mediaIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}

	items := make([]MediaSummary, 0, len(mediaRows))
	for _, m := range mediaRows {
		items = append(items, toMediaSummary(m, h.cfg))
	}

	totalPages := (total + pageSize - 1) / pageSize
	writeJSON(w, http.StatusOK, PersonMediaDTO{
		PersonID: personID,
		Media:    items,
		Page: PageInfo{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: total,
			TotalPages: totalPages,
			HasNext:    page < totalPages,
			HasPrev:    page > 1,
		},
	})
}

// GET /api/ai/status/counts — cluster/person counts for nav badge
func (h *personsHandler) statusCounts(w http.ResponseWriter, r *http.Request) {
	repo := repositories.NewPersonsRepo(h.store.DB())
	pending, err := repo.PendingClusterCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pendingClusters": pending,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	})
}

// cropURL converts an absolute crop file path to a serve URL.
// Crops are served via GET /api/ai/crops/{filename}.
func (h *personsHandler) cropURL(path string) string {
	if path == "" {
		return ""
	}
	return "/api/ai/crops/" + filepath.Base(path)
}

// GET /api/ai/crops/{filename} — serve face crop JPEG files
func (h *personsHandler) serveCrop(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if filename == "" || filename == "." || filename == ".." {
		http.NotFound(w, r)
		return
	}
	// Only allow simple filenames, no path traversal.
	if filename != filepath.Base(filename) {
		http.NotFound(w, r)
		return
	}
	cropDir := h.cfg.AI.FaceCacheDir
	if cropDir == "" {
		cropDir = filepath.Join(filepath.Dir(h.cfg.AI.ModelDir), "crops")
	}
	http.ServeFile(w, r, filepath.Join(cropDir, filename))
}

// --- helpers ---

func parseID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	s := r.PathValue(name)
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("invalid %s: %s", name, s))
		return 0, false
	}
	return id, true
}

func toMediaSummary(m repositories.Media, cfg *app.Config) MediaSummary {
	return MediaSummary{
		ID:           m.ID,
		AlbumID:      m.AlbumID,
		Filename:     m.Filename,
		Type:         m.Type,
		Width:        m.Width,
		Height:       m.Height,
		DurationMs:   m.DurationMs,
		SizeBytes:    m.SizeBytes,
		CaptureDate:  m.CaptureDate,
		ThumbnailURL: fmt.Sprintf("/api/media/%d/thumbnail", m.ID),
	}
}
