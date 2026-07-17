package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
)

// AIJobStatus values.
const (
	AIJobQueued  = "queued"
	AIJobRunning = "running"
	AIJobSuccess = "success"
	AIJobFailed  = "failed"
)

type AIJob struct {
	MediaID    int64
	Status     string
	Attempts   int
	QueuedAt   string
	FinishedAt *string
	Error      *string
}

type AILabel struct {
	ID         int64
	MediaID    int64
	Label      string
	Confidence float64
	Source     string
}

type AIFace struct {
	ID        int64
	MediaID   int64
	BBoxJSON  string
	Embedding []byte
	CropPath  string
}

type AIRepo struct {
	db *sql.DB
}

func NewAIRepo(db *sql.DB) *AIRepo {
	return &AIRepo{db: db}
}

// EnqueueNew inserts ai_jobs rows for media IDs that are not already queued/running/success.
// Existing failed jobs are reset to queued (allowing retry after binary is available).
func (r *AIRepo) EnqueueNew(mediaIDs []int64, queuedAt string) error {
	if len(mediaIDs) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO ai_jobs (media_id, status, attempts, queued_at)
		VALUES (?, 'queued', 0, ?)
		ON CONFLICT(media_id) DO UPDATE
		  SET status = 'queued', attempts = 0, queued_at = excluded.queued_at, finished_at = NULL, error = NULL
		  WHERE status = 'failed'`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, id := range mediaIDs {
		if _, err := stmt.Exec(id, queuedAt); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// EnqueueAll inserts or resets ai_jobs for every media item in the DB.
func (r *AIRepo) EnqueueAll(queuedAt string) error {
	_, err := r.db.Exec(`
		INSERT INTO ai_jobs (media_id, status, attempts, queued_at)
		SELECT id, 'queued', 0, ?
		FROM media
		ON CONFLICT(media_id) DO UPDATE
		  SET status = 'queued', attempts = 0, queued_at = excluded.queued_at, finished_at = NULL, error = NULL`,
		queuedAt)
	return err
}

// ClaimNext atomically picks the next queued job and marks it running.
// Returns nil when the queue is empty.
func (r *AIRepo) ClaimNext() (*AIJob, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	row := tx.QueryRow(`
		SELECT media_id, attempts FROM ai_jobs
		WHERE status = 'queued'
		ORDER BY queued_at ASC
		LIMIT 1`)
	var mediaID int64
	var attempts int
	if err := row.Scan(&mediaID, &attempts); err == sql.ErrNoRows {
		_ = tx.Rollback()
		return nil, nil
	} else if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if _, err := tx.Exec(`
		UPDATE ai_jobs SET status = 'running', attempts = attempts + 1
		WHERE media_id = ?`, mediaID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &AIJob{MediaID: mediaID, Status: AIJobRunning, Attempts: attempts + 1}, nil
}

// Finish marks a job success or failed.
func (r *AIRepo) Finish(mediaID int64, success bool, errMsg, finishedAt string) error {
	status := AIJobSuccess
	var errPtr *string
	if !success {
		status = AIJobFailed
		errPtr = &errMsg
	}
	_, err := r.db.Exec(`
		UPDATE ai_jobs SET status = ?, finished_at = ?, error = ?
		WHERE media_id = ?`,
		status, finishedAt, errPtr, mediaID)
	return err
}

// RequeueFailed resets failed jobs that have fewer than maxAttempts tries.
func (r *AIRepo) RequeueFailed(maxAttempts int, queuedAt string) (int64, error) {
	res, err := r.db.Exec(`
		UPDATE ai_jobs SET status = 'queued', queued_at = ?, error = NULL
		WHERE status = 'failed' AND attempts < ?`,
		queuedAt, maxAttempts)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// FailStale resets running jobs left over from a crash.
func (r *AIRepo) FailStale(finishedAt string) error {
	_, err := r.db.Exec(`
		UPDATE ai_jobs SET status = 'failed', finished_at = ?, error = 'interrupted by restart'
		WHERE status = 'running'`, finishedAt)
	return err
}

// Counts returns (queued, running, success, failed) job counts.
func (r *AIRepo) Counts() (queued, running, success, failed int, err error) {
	rows, err := r.db.Query(`
		SELECT status, COUNT(*) FROM ai_jobs GROUP BY status`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var n int
		if err = rows.Scan(&s, &n); err != nil {
			return
		}
		switch s {
		case AIJobQueued:
			queued = n
		case AIJobRunning:
			running = n
		case AIJobSuccess:
			success = n
		case AIJobFailed:
			failed = n
		}
	}
	return
}

// SaveLabels replaces all labels for a media item.
func (r *AIRepo) SaveLabels(mediaID int64, labels []AILabel) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ai_labels WHERE media_id = ?`, mediaID); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO ai_labels (media_id, label, confidence, source)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, l := range labels {
		if _, err := stmt.Exec(mediaID, l.Label, l.Confidence, l.Source); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// SaveFaces replaces all faces for a media item.
func (r *AIRepo) SaveFaces(mediaID int64, faces []AIFace) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ai_faces WHERE media_id = ?`, mediaID); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO ai_faces (media_id, bbox_json, embedding, crop_path)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, f := range faces {
		if _, err := stmt.Exec(mediaID, f.BBoxJSON, f.Embedding, f.CropPath); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// GetMediaPath returns the absolute file path for a media item.
func (r *AIRepo) GetMediaPath(mediaID int64, rootPath string) (string, string, error) {
	row := r.db.QueryRow(`SELECT relative_path, type FROM media WHERE id = ?`, mediaID)
	var relPath, mediaType string
	if err := row.Scan(&relPath, &mediaType); err != nil {
		return "", "", fmt.Errorf("media %d not found: %w", mediaID, err)
	}
	return rootPath + "/" + relPath, mediaType, nil
}

// F32ToBlob encodes a float32 slice as little-endian bytes.
func F32ToBlob(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		u := math.Float32bits(f)
		b[i*4] = byte(u)
		b[i*4+1] = byte(u >> 8)
		b[i*4+2] = byte(u >> 16)
		b[i*4+3] = byte(u >> 24)
	}
	return b
}

// BBoxToJSON serialises a bbox map to JSON.
func BBoxToJSON(x, y, w, h int) string {
	b, _ := json.Marshal(map[string]int{"x": x, "y": y, "w": w, "h": h})
	return string(b)
}
