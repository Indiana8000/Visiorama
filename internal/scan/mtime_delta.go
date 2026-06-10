package scan

import (
	"database/sql"
	"io/fs"
	"path/filepath"
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

// ComputeFolderDeltas walks the root directory and compares each album
// directory's mtime (nanoseconds) against the value stored in the albums
// table (dir_mtime_ns column).  It returns a FolderDeltaResult that the
// QuickScanner uses to decide which folders to re-scan and whether to fall
// back to FullScanner.
//
// When ignoreDirMtime is true every directory is treated as changed, which is
// required for CIFS/SMB mounts where the kernel does not update dir mtime on
// file changes.
//
// Only immediate filesystem traversal is performed; individual files are
// never stat-ed.  Hidden directories and names in excludeSet are skipped,
// matching FullScanner behaviour.
func ComputeFolderDeltas(db *sql.DB, root string, excludeSet map[string]bool, ignoreDirMtime bool) (*FolderDeltaResult, error) {
	// Load all known album mtimes from the DB keyed by relative_path.
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
	seen := make(map[string]bool, len(dbAlbums))

	// Walk the root and compare directory mtimes.
	walkErr := filepath.WalkDir(root, func(absPath string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}

		name := d.Name()
		if isExcluded(name, excludeSet) {
			return filepath.SkipDir
		}

		relPath, _ := filepath.Rel(root, absPath)
		relPath = filepath.ToSlash(relPath)
		if relPath == "." {
			relPath = ""
		}
		if relPath == "" {
			// root itself — not an album dir
			return nil
		}

		seen[relPath] = true

		info, err := d.Info()
		if err != nil {
			// Can't stat — treat as changed to be safe.
			result.ChangedDirs = append(result.ChangedDirs, relPath)
			return nil
		}
		diskMtimeNs := info.ModTime().UnixNano()

		entry, inDB := dbAlbums[relPath]
		if !inDB {
			// New folder not yet in DB — mark as changed so it gets scanned.
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

	// Detect DB albums that no longer exist on disk.
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
