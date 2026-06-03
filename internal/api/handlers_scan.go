package api

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	Mode string `json:"mode"`
}

func (h *scanHandler) trigger(w http.ResponseWriter, r *http.Request) {
	req := scanRequest{Mode: h.cfg.Scan.DefaultMode}

	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "invalid request body")
			return
		}
	}
	if req.Mode != "full" && req.Mode != "quick" {
		badRequest(w, "mode must be 'full' or 'quick'")
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

	if err := h.runner.TriggerAsync(scanID, req.Mode); err != nil {
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
