package repositories

import (
	"database/sql"
	"fmt"
	"strings"
)

type Album struct {
	ID                  int64
	RelativePath        string
	Name                string
	ParentAlbumID       *int64
	MediaCountDirect    int
	MediaCountRecursive int
	ChildAlbumCount     int
}

type AlbumsRepo struct {
	db *sql.DB
}

func NewAlbumsRepo(db *sql.DB) *AlbumsRepo {
	return &AlbumsRepo{db: db}
}

func (r *AlbumsRepo) GetRoot() (*Album, error) {
	return r.getByPath("")
}

func (r *AlbumsRepo) GetByID(id int64) (*Album, error) {
	row := r.db.QueryRow(`
		SELECT id, relative_path, name, parent_album_id,
		       media_count_direct, media_count_recursive, child_album_count
		FROM albums WHERE id = ?`, id)
	return scanAlbum(row)
}

func (r *AlbumsRepo) GetByPath(path string) (*Album, error) {
	return r.getByPath(path)
}

func (r *AlbumsRepo) getByPath(path string) (*Album, error) {
	row := r.db.QueryRow(`
		SELECT id, relative_path, name, parent_album_id,
		       media_count_direct, media_count_recursive, child_album_count
		FROM albums WHERE relative_path = ?`, path)
	return scanAlbum(row)
}

func (r *AlbumsRepo) ListChildren(parentID int64) ([]Album, error) {
	rows, err := r.db.Query(`
		SELECT id, relative_path, name, parent_album_id,
		       media_count_direct, media_count_recursive, child_album_count
		FROM albums WHERE parent_album_id = ?
		ORDER BY name`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAlbums(rows)
}

func (r *AlbumsRepo) Upsert(a *Album) (int64, error) {
	_, err := r.db.Exec(`
		INSERT INTO albums (relative_path, name, parent_album_id,
		                    media_count_direct, media_count_recursive, child_album_count)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(relative_path) DO UPDATE SET
		  name                  = excluded.name,
		  parent_album_id       = excluded.parent_album_id,
		  media_count_direct    = excluded.media_count_direct,
		  media_count_recursive = excluded.media_count_recursive,
		  child_album_count     = excluded.child_album_count`,
		a.RelativePath, a.Name, a.ParentAlbumID,
		a.MediaCountDirect, a.MediaCountRecursive, a.ChildAlbumCount)
	if err != nil {
		return 0, err
	}
	// Always SELECT — LastInsertId is unreliable for ON CONFLICT DO UPDATE
	// when the row already existed (returns stale rowid from prior session).
	var id int64
	if err := r.db.QueryRow(`SELECT id FROM albums WHERE relative_path = ?`, a.RelativePath).Scan(&id); err != nil {
		return 0, fmt.Errorf("fetch id after upsert: %w", err)
	}
	return id, nil
}

func (r *AlbumsRepo) UpdateCounts(id int64, direct, recursive, childCount int) error {
	_, err := r.db.Exec(`
		UPDATE albums SET media_count_direct = ?, media_count_recursive = ?, child_album_count = ?
		WHERE id = ?`, direct, recursive, childCount, id)
	return err
}

func (r *AlbumsRepo) Breadcrumbs(albumID int64) ([]Album, error) {
	var chain []Album
	id := &albumID
	for id != nil {
		a, err := r.GetByID(*id)
		if err != nil {
			return nil, err
		}
		chain = append([]Album{*a}, chain...)
		id = a.ParentAlbumID
	}
	return chain, nil
}

// ListAllPaths returns every relative_path stored in the albums table (excluding root).
func (r *AlbumsRepo) ListAllPaths() ([]string, error) {
	rows, err := r.db.Query(`SELECT relative_path FROM albums WHERE relative_path != ''`)
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

// ListAllPathIDs returns relative_path → id for every album including root.
func (r *AlbumsRepo) ListAllPathIDs() (map[string]int64, error) {
	rows, err := r.db.Query(`SELECT relative_path, id FROM albums`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int64{}
	for rows.Next() {
		var path string
		var id int64
		if err := rows.Scan(&path, &id); err != nil {
			return nil, err
		}
		m[path] = id
	}
	return m, rows.Err()
}

// ListOrphanPaths returns album paths (excluding root) NOT in the _seen_albums temp table.
// db must be the same *sql.DB that created the temp table.
func (r *AlbumsRepo) ListOrphanPaths(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT relative_path FROM albums WHERE relative_path != '' AND relative_path NOT IN (SELECT path FROM _seen_albums)`)
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

// DeleteByPath removes an album and all its media (cascade via FK not guaranteed in SQLite
// without ON DELETE CASCADE, so we delete media first).
func (r *AlbumsRepo) DeleteByPath(relPath string) error {
	row := r.db.QueryRow(`SELECT id FROM albums WHERE relative_path = ?`, relPath)
	var id int64
	if err := row.Scan(&id); err != nil {
		return nil // already gone
	}
	if _, err := r.db.Exec(`DELETE FROM media WHERE album_id = ?`, id); err != nil {
		return err
	}
	_, err := r.db.Exec(`DELETE FROM albums WHERE id = ?`, id)
	return err
}

func (r *AlbumsRepo) CoverMediaID(albumID int64) (*int64, error) {
	return r.coverMediaIDRecursive(albumID, 0)
}

// coverMediaIDRecursive finds the first media item in albumID, then recursively
// searches child albums (breadth-first, alphabetical) when no direct media exists.
// depth guards against cycles from corrupt parent_album_id data.
func (r *AlbumsRepo) coverMediaIDRecursive(albumID int64, depth int) (*int64, error) {
	if depth > 32 {
		return nil, nil
	}
	row := r.db.QueryRow(`
		SELECT id FROM media WHERE album_id = ? ORDER BY filename ASC LIMIT 1`, albumID)
	var id int64
	if err := row.Scan(&id); err == nil {
		return &id, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	// No direct media — recurse into child albums ordered by name.
	children, err := r.ListChildren(albumID)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if coverID, err := r.coverMediaIDRecursive(child.ID, depth+1); err == nil && coverID != nil {
			return coverID, nil
		}
	}
	return nil, nil
}

type AlbumMatchRow struct {
	Album
	MatchCount int
}

// AlbumsByMediaIDs returns albums that contain at least one of the given media IDs,
// with a count of how many of the given IDs belong to each album, sorted descending.
func (r *AlbumsRepo) AlbumsByMediaIDs(ids []int64) ([]AlbumMatchRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT a.id, a.relative_path, a.name, a.parent_album_id,
		       a.media_count_direct, a.media_count_recursive, a.child_album_count,
		       COUNT(m.id) AS match_count
		FROM albums a
		JOIN media m ON m.album_id = a.id AND m.id IN (`+placeholders+`)
		GROUP BY a.id
		ORDER BY match_count DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AlbumMatchRow
	for rows.Next() {
		var row AlbumMatchRow
		if err := rows.Scan(&row.ID, &row.RelativePath, &row.Name, &row.ParentAlbumID,
			&row.MediaCountDirect, &row.MediaCountRecursive, &row.ChildAlbumCount,
			&row.MatchCount); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func scanAlbum(row *sql.Row) (*Album, error) {
	a := &Album{}
	err := row.Scan(&a.ID, &a.RelativePath, &a.Name, &a.ParentAlbumID,
		&a.MediaCountDirect, &a.MediaCountRecursive, &a.ChildAlbumCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func collectAlbums(rows *sql.Rows) ([]Album, error) {
	var out []Album
	for rows.Next() {
		var a Album
		if err := rows.Scan(&a.ID, &a.RelativePath, &a.Name, &a.ParentAlbumID,
			&a.MediaCountDirect, &a.MediaCountRecursive, &a.ChildAlbumCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
