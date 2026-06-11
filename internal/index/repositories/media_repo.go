package repositories

import (
	"database/sql"
	"fmt"
)

type Media struct {
	ID           int64
	AlbumID      int64
	Filename     string
	RelativePath string
	Type         string
	Width        *int
	Height       *int
	DurationMs   *int64
	SizeBytes    int64
	CaptureDate  *string
	Extension    string
	MimeType     string
	CameraModel  *string
	LensModel    *string
	GpsLat       *float64
	GpsLon       *float64
	Orientation  *int
	MtimeUnix    int64
}

type MediaRepo struct {
	db *sql.DB
}

func NewMediaRepo(db *sql.DB) *MediaRepo {
	return &MediaRepo{db: db}
}

func (r *MediaRepo) GetByID(id int64) (*Media, error) {
	row := r.db.QueryRow(`
		SELECT id, album_id, filename, relative_path, type,
		       width, height, duration_ms, size_bytes, capture_date,
		       extension, mime_type, camera_model, lens_model,
		       gps_lat, gps_lon, orientation, mtime_unix
		FROM media WHERE id = ?`, id)
	return scanMedia(row)
}

func (r *MediaRepo) ListByAlbum(albumID int64, offset, limit int) ([]Media, error) {
	rows, err := r.db.Query(`
		SELECT id, album_id, filename, relative_path, type,
		       width, height, duration_ms, size_bytes, capture_date,
		       extension, mime_type, camera_model, lens_model,
		       gps_lat, gps_lon, orientation, mtime_unix
		FROM media WHERE album_id = ?
		LIMIT ? OFFSET ?`, albumID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectMedia(rows)
}

func (r *MediaRepo) CountByAlbum(albumID int64) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM media WHERE album_id = ?`, albumID).Scan(&n)
	return n, err
}

func (r *MediaRepo) Upsert(m *Media) (int64, error) {
	res, err := r.db.Exec(`
		INSERT INTO media (album_id, filename, relative_path, type,
		                   width, height, duration_ms, size_bytes, capture_date,
		                   extension, mime_type, camera_model, lens_model,
		                   gps_lat, gps_lon, orientation, mtime_unix)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(relative_path) DO UPDATE SET
		  album_id     = excluded.album_id,
		  filename     = excluded.filename,
		  type         = excluded.type,
		  width        = excluded.width,
		  height       = excluded.height,
		  duration_ms  = excluded.duration_ms,
		  size_bytes   = excluded.size_bytes,
		  capture_date = excluded.capture_date,
		  extension    = excluded.extension,
		  mime_type    = excluded.mime_type,
		  camera_model = excluded.camera_model,
		  lens_model   = excluded.lens_model,
		  gps_lat      = excluded.gps_lat,
		  gps_lon      = excluded.gps_lon,
		  orientation  = excluded.orientation,
		  mtime_unix   = excluded.mtime_unix`,
		m.AlbumID, m.Filename, m.RelativePath, m.Type,
		m.Width, m.Height, m.DurationMs, m.SizeBytes, m.CaptureDate,
		m.Extension, m.MimeType, m.CameraModel, m.LensModel,
		m.GpsLat, m.GpsLon, m.Orientation, m.MtimeUnix)
	if err != nil {
		return 0, err
	}
	_ = res
	var id int64
	if err := r.db.QueryRow(`SELECT id FROM media WHERE relative_path = ?`, m.RelativePath).Scan(&id); err != nil {
		return 0, fmt.Errorf("fetch id after upsert: %w", err)
	}
	return id, nil
}

func (r *MediaRepo) DeleteByPath(relativePath string) error {
	_, err := r.db.Exec(`DELETE FROM media WHERE relative_path = ?`, relativePath)
	return err
}

// ListPathsByAlbum returns all relative_paths for media in a given album.
func (r *MediaRepo) ListPathsByAlbum(albumID int64) ([]string, error) {
	rows, err := r.db.Query(`SELECT relative_path FROM media WHERE album_id = ?`, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// ListAllPaths returns every relative_path stored in the media table.
func (r *MediaRepo) ListAllPaths() ([]string, error) {
	rows, err := r.db.Query(`SELECT relative_path FROM media`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// ListOrphanPaths returns media paths that are NOT in the _seen_media temp table.
// db must be the same *sql.DB that created the temp table.
func (r *MediaRepo) ListOrphanPaths(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT relative_path FROM media WHERE relative_path NOT IN (SELECT path FROM _seen_media)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// GetThumbReady returns true if the item has thumb_ready = 1.
func (r *MediaRepo) GetThumbReady(id int64) (bool, error) {
	var v int
	err := r.db.QueryRow(`SELECT thumb_ready FROM media WHERE id = ?`, id).Scan(&v)
	return v == 1, err
}

// SetThumbReady marks a media item's thumb_ready flag.
func (r *MediaRepo) SetThumbReady(id int64, ready bool) error {
	v := 0
	if ready {
		v = 1
	}
	_, err := r.db.Exec(`UPDATE media SET thumb_ready = ? WHERE id = ?`, v, id)
	return err
}

// NextThumbPending returns the next media item with thumb_ready = 0, or nil if none.
func (r *MediaRepo) NextThumbPending() (*Media, error) {
	row := r.db.QueryRow(`
		SELECT id, album_id, filename, relative_path, type,
		       width, height, duration_ms, size_bytes, capture_date,
		       extension, mime_type, camera_model, lens_model,
		       gps_lat, gps_lon, orientation, mtime_unix
		FROM media WHERE thumb_ready = 0 LIMIT 1`)
	m, err := scanMedia(row)
	return m, err
}

// CountThumbPending returns how many media items still need thumbnail generation.
func (r *MediaRepo) CountThumbPending() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM media WHERE thumb_ready = 0`).Scan(&n)
	return n, err
}

// ResetAllThumbReady sets thumb_ready = 0 for all media items and returns the count affected.
func (r *MediaRepo) ResetAllThumbReady() (int64, error) {
	res, err := r.db.Exec(`UPDATE media SET thumb_ready = 0`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *MediaRepo) GetMtimeByPath(relativePath string) (int64, bool, error) {
	var mtime int64
	err := r.db.QueryRow(`SELECT mtime_unix FROM media WHERE relative_path = ?`, relativePath).Scan(&mtime)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return mtime, true, err
}

// GPSMedia holds the minimal GPS data needed for map clustering.
type GPSMedia struct {
	ID     int64
	GpsLat float64
	GpsLon float64
}

// GetGPSMedia returns all media with GPS coordinates.
// If albumID is non-nil, only media in that album and all its sub-albums are returned.
func (r *MediaRepo) GetGPSMedia(albumID *int64) ([]GPSMedia, error) {
	if albumID == nil {
		rows, err := r.db.Query(`
            SELECT id, gps_lat, gps_lon FROM media
            WHERE gps_lat IS NOT NULL AND gps_lon IS NOT NULL`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return collectGPSMedia(rows)
	}
	// Recursive CTE to get all sub-album IDs including the root album
	rows, err := r.db.Query(`
        WITH RECURSIVE sub(id) AS (
            SELECT id FROM albums WHERE id = ?
            UNION ALL
            SELECT a.id FROM albums a JOIN sub s ON a.parent_album_id = s.id
        )
        SELECT m.id, m.gps_lat, m.gps_lon
        FROM media m
        JOIN sub s ON m.album_id = s.id
        WHERE m.gps_lat IS NOT NULL AND m.gps_lon IS NOT NULL`, *albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectGPSMedia(rows)
}

// CountGPSMedia returns how many media items have GPS coordinates in a given album tree.
// albumID nil = global count.
func (r *MediaRepo) CountGPSMedia(albumID *int64) (int, error) {
	if albumID == nil {
		var n int
		err := r.db.QueryRow(`SELECT COUNT(*) FROM media WHERE gps_lat IS NOT NULL AND gps_lon IS NOT NULL`).Scan(&n)
		return n, err
	}
	var n int
	err := r.db.QueryRow(`
        WITH RECURSIVE sub(id) AS (
            SELECT id FROM albums WHERE id = ?
            UNION ALL
            SELECT a.id FROM albums a JOIN sub s ON a.parent_album_id = s.id
        )
        SELECT COUNT(*) FROM media m JOIN sub s ON m.album_id = s.id
        WHERE m.gps_lat IS NOT NULL AND m.gps_lon IS NOT NULL`, *albumID).Scan(&n)
	return n, err
}

func collectGPSMedia(rows *sql.Rows) ([]GPSMedia, error) {
	var out []GPSMedia
	for rows.Next() {
		var g GPSMedia
		if err := rows.Scan(&g.ID, &g.GpsLat, &g.GpsLon); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func scanMedia(row *sql.Row) (*Media, error) {
	m := &Media{}
	err := row.Scan(
		&m.ID, &m.AlbumID, &m.Filename, &m.RelativePath, &m.Type,
		&m.Width, &m.Height, &m.DurationMs, &m.SizeBytes, &m.CaptureDate,
		&m.Extension, &m.MimeType, &m.CameraModel, &m.LensModel,
		&m.GpsLat, &m.GpsLon, &m.Orientation, &m.MtimeUnix,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

func collectMedia(rows *sql.Rows) ([]Media, error) {
	var out []Media
	for rows.Next() {
		var m Media
		if err := rows.Scan(
			&m.ID, &m.AlbumID, &m.Filename, &m.RelativePath, &m.Type,
			&m.Width, &m.Height, &m.DurationMs, &m.SizeBytes, &m.CaptureDate,
			&m.Extension, &m.MimeType, &m.CameraModel, &m.LensModel,
			&m.GpsLat, &m.GpsLon, &m.Orientation, &m.MtimeUnix,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
