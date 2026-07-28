package repositories

import (
	"database/sql"
	"log/slog"
	"time"
)

// scanJobRetention bounds how long finished scan_jobs/scan_errors rows are
// kept; without this both tables grow forever since nothing else prunes them.
const scanJobRetention = 90 * 24 * time.Hour

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
	if err != nil {
		return err
	}
	if pruneErr := r.PruneOlderThan(time.Now().UTC().Add(-scanJobRetention)); pruneErr != nil {
		slog.Warn("scan_jobs: prune old rows failed", "err", pruneErr)
	}
	return nil
}

// PruneOlderThan deletes finished scan_jobs (and their scan_errors) whose
// finished_at is before cutoff. Queued/running jobs are never pruned.
func (r *ScanRepo) PruneOlderThan(cutoff time.Time) error {
	cutoffStr := cutoff.Format(time.RFC3339)
	if _, err := r.db.Exec(`
		DELETE FROM scan_errors WHERE scan_id IN (
			SELECT id FROM scan_jobs
			WHERE status IN ('success','failed') AND finished_at IS NOT NULL AND finished_at < ?
		)`, cutoffStr); err != nil {
		return err
	}
	_, err := r.db.Exec(`
		DELETE FROM scan_jobs
		WHERE status IN ('success','failed') AND finished_at IS NOT NULL AND finished_at < ?`,
		cutoffStr)
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

func (r *ScanRepo) GetAll(limit int) ([]*ScanJob, error) {
	rows, err := r.db.Query(`
		SELECT id, mode, status, started_at, finished_at,
		       scanned_files, indexed_files, skipped_files,
		       error_count, fallback_to_full
		FROM scan_jobs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*ScanJob
	for rows.Next() {
		j := &ScanJob{}
		var fallback int
		if err := rows.Scan(&j.ID, &j.Mode, &j.Status, &j.StartedAt, &j.FinishedAt,
			&j.ScannedFiles, &j.IndexedFiles, &j.SkippedFiles, &j.ErrorCount, &fallback); err != nil {
			return nil, err
		}
		j.FallbackToFull = fallback == 1
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
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

// FailStale marks any queued/running jobs as failed with the given finishedAt timestamp.
// Called at startup to clean up jobs left hanging by a previous crash.
func (r *ScanRepo) FailStale(finishedAt string) error {
	_, err := r.db.Exec(`
		UPDATE scan_jobs SET status = 'failed', finished_at = ?
		WHERE status IN ('queued', 'running')`, finishedAt)
	return err
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
