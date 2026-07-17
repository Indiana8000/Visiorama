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

// EnqueueForAlbum re-queues all media in the given album path (non-recursive) for AI analysis.
func (r *AIRepo) EnqueueForAlbum(albumPath, queuedAt string) error {
	_, err := r.db.Exec(`
		INSERT INTO ai_jobs (media_id, status, attempts, queued_at)
		SELECT m.id, 'queued', 0, ?
		FROM media m
		JOIN albums a ON a.id = m.album_id
		WHERE a.relative_path = ?
		ON CONFLICT(media_id) DO UPDATE
		  SET status = 'queued', attempts = 0, queued_at = excluded.queued_at,
		      finished_at = NULL, error = NULL`,
		queuedAt, albumPath)
	return err
}

// DeleteOrphanedAIData removes labels, faces and jobs for media that no longer exists.
func (r *AIRepo) DeleteOrphanedAIData() (int64, error) {
	var total int64
	for _, tbl := range []string{"ai_labels", "ai_faces", "ai_jobs"} {
		res, err := r.db.Exec(`DELETE FROM ` + tbl + ` WHERE media_id NOT IN (SELECT id FROM media)`)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
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

// SaveFaces replaces all faces for a media item while preserving confirmed person assignments.
// Confirmed faces are matched to new detections by nearest BBox center; unmatched confirmed
// faces keep their old embedding/crop. Faces without confirmed assignments are fully replaced.
func (r *AIRepo) SaveFaces(mediaID int64, faces []AIFace) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	// Load existing faces that have a confirmed assignment — these must not lose their person link.
	rows, err := tx.Query(`
		SELECT f.id, f.bbox_json FROM ai_faces f
		JOIN ai_face_assignments a ON a.face_id = f.id AND a.confirmed = 1
		WHERE f.media_id = ?`, mediaID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	var confirmed []confirmedFace
	for rows.Next() {
		var cf confirmedFace
		if err := rows.Scan(&cf.id, &cf.bboxJSON); err != nil {
			rows.Close()
			_ = tx.Rollback()
			return err
		}
		confirmed = append(confirmed, cf)
	}
	rows.Close()

	// Delete only unconfirmed faces.
	if _, err := tx.Exec(`
		DELETE FROM ai_faces WHERE media_id = ?
		  AND id NOT IN (
		    SELECT face_id FROM ai_face_assignments WHERE confirmed = 1
		  )`, mediaID); err != nil {
		_ = tx.Rollback()
		return err
	}

	// For each new detection, check if it overlaps a confirmed face (BBox IoU).
	// Matched confirmed faces get their embedding + crop updated in-place.
	// Unmatched new detections are inserted fresh.
	matched := make(map[int64]bool)
	for _, f := range faces {
		bestID, bestIoU := matchBBox(f.BBoxJSON, confirmed)
		if bestIoU > 0.3 && !matched[bestID] {
			matched[bestID] = true
			if _, err := tx.Exec(`
				UPDATE ai_faces SET bbox_json = ?, embedding = ?, crop_path = ? WHERE id = ?`,
				f.BBoxJSON, f.Embedding, f.CropPath, bestID); err != nil {
				_ = tx.Rollback()
				return err
			}
		} else {
			if _, err := tx.Exec(`
				INSERT INTO ai_faces (media_id, bbox_json, embedding, crop_path)
				VALUES (?, ?, ?, ?)`, mediaID, f.BBoxJSON, f.Embedding, f.CropPath); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit()
}

type confirmedFace struct {
	id       int64
	bboxJSON string
}

// matchBBox finds the confirmed face with highest IoU to bboxJSON. Returns id and IoU.
func matchBBox(bboxJSON string, confirmed []confirmedFace) (int64, float64) {
	ax, ay, aw, ah := parseBBox(bboxJSON)
	var bestID int64
	var bestIoU float64
	for _, cf := range confirmed {
		bx, by, bw, bh := parseBBox(cf.bboxJSON)
		iou := bboxIoU(ax, ay, aw, ah, bx, by, bw, bh)
		if iou > bestIoU {
			bestIoU = iou
			bestID = cf.id
		}
	}
	return bestID, bestIoU
}

func parseBBox(s string) (x, y, w, h float64) {
	// {"x":10,"y":20,"w":30,"h":40}
	var m map[string]float64
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return 0, 0, 0, 0
	}
	return m["x"], m["y"], m["w"], m["h"]
}

func bboxIoU(ax, ay, aw, ah, bx, by, bw, bh float64) float64 {
	ix1 := max64(ax, bx)
	iy1 := max64(ay, by)
	ix2 := min64(ax+aw, bx+bw)
	iy2 := min64(ay+ah, by+bh)
	if ix2 <= ix1 || iy2 <= iy1 {
		return 0
	}
	inter := (ix2 - ix1) * (iy2 - iy1)
	union := aw*ah + bw*bh - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
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

// MediaAILabel is a label row for one media item.
type MediaAILabel struct {
	Label      string
	Confidence float64
	Source     string
}

// MediaAIFace is a face + assigned person for one media item.
type MediaAIFace struct {
	FaceID    int64
	CropPath  string
	BBoxJSON  string
	PersonID  *int64
	PersonName *string
}

// GetMediaAI returns all labels and faces (with person assignment) for one media item.
func (r *AIRepo) GetMediaAI(mediaID int64) (labels []MediaAILabel, faces []MediaAIFace, err error) {
	rows, err := r.db.Query(
		`SELECT label, confidence, source FROM ai_labels WHERE media_id = ? ORDER BY confidence DESC`,
		mediaID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l MediaAILabel
		if err := rows.Scan(&l.Label, &l.Confidence, &l.Source); err != nil {
			return nil, nil, err
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	frows, err := r.db.Query(`
		SELECT f.id, f.crop_path, f.bbox_json, p.id, p.name
		FROM ai_faces f
		LEFT JOIN ai_face_assignments a ON a.face_id = f.id AND a.confirmed = 1
		LEFT JOIN ai_persons p ON p.id = a.person_id
		WHERE f.media_id = ?
		ORDER BY f.id`, mediaID)
	if err != nil {
		return nil, nil, err
	}
	defer frows.Close()
	for frows.Next() {
		var f MediaAIFace
		if err := frows.Scan(&f.FaceID, &f.CropPath, &f.BBoxJSON, &f.PersonID, &f.PersonName); err != nil {
			return nil, nil, err
		}
		faces = append(faces, f)
	}
	return labels, faces, frows.Err()
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
