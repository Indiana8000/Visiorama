package repositories

import (
	"database/sql"
)

type ScanJob struct {
	ID             string
	Mode           string
	Status         string
	StartedAt      *string
	FinishedAt     *string
	ScannedFiles   int
	IndexedFiles   int
	SkippedFiles   int
	ErrorCount     int
	FallbackToFull bool
}

type ScanRepo struct {
	db *sql.DB
}

func NewScanRepo(db *sql.DB) *ScanRepo {
	return &ScanRepo{db: db}
}

func (r *ScanRepo) Create(job *ScanJob) error {
	_, err := r.db.Exec(`
		INSERT INTO scan_jobs (id, mode, status, started_at, finished_at,
		                       scanned_files, indexed_files, skipped_files,
		                       error_count, fallback_to_full)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Mode, job.Status, job.StartedAt, job.FinishedAt,
		job.ScannedFiles, job.IndexedFiles, job.SkippedFiles,
		job.ErrorCount, boolToInt(job.FallbackToFull))
	return err
}

func (r *ScanRepo) GetByID(id string) (*ScanJob, error) {
	row := r.db.QueryRow(`
		SELECT id, mode, status, started_at, finished_at,
		       scanned_files, indexed_files, skipped_files,
		       error_count, fallback_to_full
		FROM scan_jobs WHERE id = ?`, id)
	return scanJob(row)
}

func (r *ScanRepo) UpdateStatus(id, status string, startedAt, finishedAt *string) error {
	_, err := r.db.Exec(`
		UPDATE scan_jobs SET status = ?, started_at = ?, finished_at = ? WHERE id = ?`,
		status, startedAt, finishedAt, id)
	return err
}

func (r *ScanRepo) UpdateCounters(id string, scanned, indexed, skipped, errCount int, fallback bool) error {
	_, err := r.db.Exec(`
		UPDATE scan_jobs SET scanned_files = ?, indexed_files = ?, skipped_files = ?,
		                     error_count = ?, fallback_to_full = ?
		WHERE id = ?`,
		scanned, indexed, skipped, errCount, boolToInt(fallback), id)
	return err
}

// GetActive returns the currently queued or running scan job, or nil if none.
func (r *ScanRepo) GetActive() (*ScanJob, error) {
	row := r.db.QueryRow(`
		SELECT id, mode, status, started_at, finished_at,
		       scanned_files, indexed_files, skipped_files,
		       error_count, fallback_to_full
		FROM scan_jobs WHERE status IN ('queued','running')
		ORDER BY started_at DESC LIMIT 1`)
	return scanJob(row)
}

func (r *ScanRepo) HasRunning() (bool, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM scan_jobs WHERE status IN ('queued','running')`).Scan(&n)
	return n > 0, err
}

func (r *ScanRepo) AddError(scanID, path, errMsg string) error {
	_, err := r.db.Exec(`
		INSERT INTO scan_errors (scan_id, path, error) VALUES (?, ?, ?)`,
		scanID, path, errMsg)
	return err
}

func scanJob(row *sql.Row) (*ScanJob, error) {
	j := &ScanJob{}
	var fallback int
	err := row.Scan(&j.ID, &j.Mode, &j.Status, &j.StartedAt, &j.FinishedAt,
		&j.ScannedFiles, &j.IndexedFiles, &j.SkippedFiles, &j.ErrorCount, &fallback)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	j.FallbackToFull = fallback == 1
	return j, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
