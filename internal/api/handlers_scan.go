package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/scan"
)

type scanHandler struct {
	cfg    *app.Config
	store  *index.Store
	runner *scan.Runner
}

type scanRequest struct {
	Mode      string `json:"mode"`
	AlbumPath string `json:"albumPath"` // optional: relative path to scan subtree
}

func (h *scanHandler) trigger(w http.ResponseWriter, r *http.Request) {
	req := scanRequest{Mode: h.cfg.Scan.DefaultMode}

	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "invalid request body")
			return
		}
	}
	if req.Mode != "full" && req.Mode != "quick" && req.Mode != "orphan" {
		badRequest(w, "mode must be 'full', 'quick' or 'orphan'")
		return
	}

	// Sanitise albumPath: must be clean, relative, no escaping root.
	albumPath := filepath.ToSlash(filepath.Clean(req.AlbumPath))
	if albumPath == "." {
		albumPath = ""
	}
	if strings.HasPrefix(albumPath, "..") || filepath.IsAbs(albumPath) {
		badRequest(w, "albumPath must be a relative path inside the library")
		return
	}

	if h.runner.IsRunning() {
		writeError(w, http.StatusConflict, "SCAN_ALREADY_RUNNING", "a scan job is already in progress")
		return
	}

	scanID := fmt.Sprintf("scan-%d", time.Now().UnixMilli())
	scanRepo := repositories.NewScanRepo(h.store.DB())
	job := &repositories.ScanJob{
		ID:     scanID,
		Mode:   req.Mode,
		Status: "queued",
	}
	if err := scanRepo.Create(job); err != nil {
		internalError(w)
		return
	}

	if err := h.runner.TriggerAsync(scanID, req.Mode, albumPath); err != nil {
		writeError(w, http.StatusConflict, "SCAN_ALREADY_RUNNING", "a scan job is already in progress")
		return
	}

	writeJSON(w, http.StatusAccepted, repoScanJobToDTO(job))
}

func (h *scanHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	scanID := r.PathValue("scanId")
	scanRepo := repositories.NewScanRepo(h.store.DB())
	job, err := scanRepo.GetByID(scanID)
	if err != nil || job == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, repoScanJobToDTO(job))
}

func (h *scanHandler) getAll(w http.ResponseWriter, r *http.Request) {
	scanRepo := repositories.NewScanRepo(h.store.DB())
	jobs, err := scanRepo.GetAll(100)
	if err != nil {
		internalError(w)
		return
	}
	dtos := make([]ScanJob, len(jobs))
	for i, j := range jobs {
		dtos[i] = repoScanJobToDTO(j)
	}
	writeJSON(w, http.StatusOK, dtos)
}

func (h *scanHandler) getActive(w http.ResponseWriter, r *http.Request) {
	scanRepo := repositories.NewScanRepo(h.store.DB())
	job, err := scanRepo.GetActive()
	if err != nil || job == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, repoScanJobToDTO(job))
}

func repoScanJobToDTO(j *repositories.ScanJob) ScanJob {
	return ScanJob{
		ID:             j.ID,
		Mode:           j.Mode,
		Status:         j.Status,
		StartedAt:      j.StartedAt,
		FinishedAt:     j.FinishedAt,
		ScannedFiles:   j.ScannedFiles,
		IndexedFiles:   j.IndexedFiles,
		SkippedFiles:   j.SkippedFiles,
		ErrorCount:     j.ErrorCount,
		FallbackToFull: j.FallbackToFull,
	}
}
