package repositories

import (
	"database/sql"
)

type TranscodeJob struct {
	ID         string
	MediaID    int64
	Status     string
	OutputPath *string
	Error      *string
	CreatedAt  string
	FinishedAt *string
}

type TranscodeRepo struct {
	db *sql.DB
}

func NewTranscodeRepo(db *sql.DB) *TranscodeRepo {
	return &TranscodeRepo{db: db}
}

func (r *TranscodeRepo) Create(job *TranscodeJob) error {
	_, err := r.db.Exec(`
		INSERT INTO transcode_jobs (id, media_id, status, output_path, error, created_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.MediaID, job.Status, job.OutputPath, job.Error, job.CreatedAt, job.FinishedAt)
	return err
}

func (r *TranscodeRepo) GetByID(id string) (*TranscodeJob, error) {
	row := r.db.QueryRow(`
		SELECT id, media_id, status, output_path, error, created_at, finished_at
		FROM transcode_jobs WHERE id = ?`, id)
	return scanTranscodeJob(row)
}

func (r *TranscodeRepo) GetLatestForMedia(mediaID int64) (*TranscodeJob, error) {
	row := r.db.QueryRow(`
		SELECT id, media_id, status, output_path, error, created_at, finished_at
		FROM transcode_jobs WHERE media_id = ?
		ORDER BY created_at DESC LIMIT 1`, mediaID)
	return scanTranscodeJob(row)
}

func (r *TranscodeRepo) UpdateStatus(id, status string, outputPath, errMsg, finishedAt *string) error {
	_, err := r.db.Exec(`
		UPDATE transcode_jobs SET status = ?, output_path = ?, error = ?, finished_at = ?
		WHERE id = ?`,
		status, outputPath, errMsg, finishedAt, id)
	return err
}

func (r *TranscodeRepo) FailStale(finishedAt string) error {
	_, err := r.db.Exec(`
		UPDATE transcode_jobs SET status = 'failed', finished_at = ?, error = 'interrupted by server restart'
		WHERE status IN ('queued','running')`, finishedAt)
	return err
}

// DeleteExpired removes jobs (and returns output paths) older than the given createdAt threshold.
func (r *TranscodeRepo) DeleteExpired(before string) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT output_path FROM transcode_jobs
		WHERE created_at < ? AND output_path IS NOT NULL`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil && p != "" {
			paths = append(paths, p)
		}
	}
	_, err = r.db.Exec(`DELETE FROM transcode_jobs WHERE created_at < ?`, before)
	return paths, err
}

func scanTranscodeJob(row *sql.Row) (*TranscodeJob, error) {
	j := &TranscodeJob{}
	err := row.Scan(&j.ID, &j.MediaID, &j.Status, &j.OutputPath, &j.Error, &j.CreatedAt, &j.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}
