package scan

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"strings"
)

// FolderDeltaResult is returned by ComputeFolderDeltas.
type FolderDeltaResult struct {
	// ChangedDirs contains relative paths of album directories whose mtime
	// differs from the value stored in the DB.
	ChangedDirs []string

	// DBEmpty is true when no albums exist in the DB yet (first run).
	DBEmpty bool

	// DeletedDirs contains relative paths of album directories that exist in
	// the DB but were not found on disk.  Their presence makes quick-scan
	// unreliable, so the caller should fall back to a full scan.
	DeletedDirs []string
}

// ComputeFolderDeltas walks libraryRoot/albumPath and compares each album
// directory's mtime against the DB.  albumPath="" means the entire library.
// Only DB albums inside albumPath are considered, so sibling albums are never
// falsely reported as deleted when scanning a subtree.
//
// When ignoreDirMtime is true every directory is treated as changed.
func ComputeFolderDeltas(db *sql.DB, libraryRoot string, albumPath string, excludeSet map[string]bool, ignoreDirMtime bool) (*FolderDeltaResult, error) {
	walkRoot := libraryRoot
	if albumPath != "" {
		walkRoot = filepath.Join(libraryRoot, filepath.FromSlash(albumPath))
	}

	// Load DB album mtimes; only keep entries inside albumPath.
	rows, err := db.Query(`SELECT relative_path, dir_mtime_ns FROM albums WHERE relative_path != ''`)
	if err != nil {
		return nil, err
	}
	type dbEntry struct {
		mtimeNs *int64
	}
	dbAlbums := map[string]dbEntry{}
	for rows.Next() {
		var relPath string
		var mtimeNs *int64
		if err := rows.Scan(&relPath, &mtimeNs); err != nil {
			rows.Close()
			return nil, err
		}
		// When scanning a subtree, ignore albums outside it.
		if albumPath != "" && !strings.HasPrefix(relPath, albumPath+"/") && relPath != albumPath {
			continue
		}
		dbAlbums[relPath] = dbEntry{mtimeNs: mtimeNs}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &FolderDeltaResult{}
	if len(dbAlbums) == 0 {
		result.DBEmpty = true
		return result, nil
	}

	// Track which DB albums we have seen on disk.
	// Pre-mark the walk root itself as seen — the walk callback skips it.
	seen := make(map[string]bool, len(dbAlbums))
	if albumPath != "" {
		seen[albumPath] = true
	}

	// Walk and compare directory mtimes. relPaths are always relative to libraryRoot.
	walkErr := filepath.WalkDir(walkRoot, func(absPath string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}

		name := d.Name()
		if isExcluded(name, excludeSet) {
			return filepath.SkipDir
		}

		relPath, _ := filepath.Rel(libraryRoot, absPath)
		relPath = filepath.ToSlash(relPath)
		if relPath == "." {
			relPath = ""
		}
		if relPath == "" || relPath == albumPath {
			// walkRoot itself is an existing album — skip the dir entry, walk contents.
			return nil
		}

		seen[relPath] = true

		info, err := d.Info()
		if err != nil {
			result.ChangedDirs = append(result.ChangedDirs, relPath)
			return nil
		}
		diskMtimeNs := info.ModTime().UnixNano()

		entry, inDB := dbAlbums[relPath]
		if !inDB {
			result.ChangedDirs = append(result.ChangedDirs, relPath)
			return nil
		}
		if ignoreDirMtime || entry.mtimeNs == nil || *entry.mtimeNs != diskMtimeNs {
			result.ChangedDirs = append(result.ChangedDirs, relPath)
		}

		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// Detect DB albums (within scope) that no longer exist on disk.
	for relPath := range dbAlbums {
		if !seen[relPath] {
			result.DeletedDirs = append(result.DeletedDirs, relPath)
		}
	}

	return result, nil
}

// UpdateDirMtimeNs persists the directory mtime (nanoseconds) for a single
// album identified by its relative path.
func UpdateDirMtimeNs(db *sql.DB, relPath string, mtimeNs int64) error {
	_, err := db.Exec(`UPDATE albums SET dir_mtime_ns = ? WHERE relative_path = ?`, mtimeNs, relPath)
	return err
}
