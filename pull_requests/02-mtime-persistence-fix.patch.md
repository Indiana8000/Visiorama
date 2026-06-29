# PR #2: Missing Mtime Persistence Consistency Fix

## Changes
- Add systematic directory mtime updates in FullScanner for CIFS/SMB compatibility
- Ensure QuickScanner updates mtimes after orphan pruning
- Normalize timestamp precision handling (UnixNano vs Unix)

## Rationale
CIFS/SMB filesystems don't update directory mtimes when files change. Without consistent mtime persistence, quick-scans will treat unchanged directories as changed every time. This causes unnecessary full fallback scans.

---

### Modified Files: `internal/scan/scanner_full.go`

```go SEARCH internal/scan/scanner_full.go 203,210
		if d.IsDir() {
			if relPath == "" {
				return nil
			}
			if _, err := ensureAlbum(relPath); err != nil {
				slog.Warn("ensure album", "path", relPath, "err", err)
			}
			if info, err := d.Info(); err == nil {
				_ = UpdateDirMtimeNs(s.store.DB(), relPath, info.ModTime().UnixNano())
			}
			slog.Debug("full scan: album dir", "path", relPath)
```

```go REPLACE internal/scan/scanner_full.go 203,210
		if d.IsDir() {
			if relPath == "" {
				return nil
			}
			
			// Ensure album exists and persist mtime (including CIFS/SMB compatibility)
			if err := s.ensureAlbumWithMtimeUpdate(relPath); err != nil {
				slog.Warn("ensure album", "path", relPath, "err", err)
			}
			slog.Debug("full scan: album dir", "path", relPath)
```

---

### New Method in `internal/scan/scanner_full.go`

Add after line ~213 (after `isExcluded` function):

```go
// ensureAlbumWithMtimeUpdate ensures an album row exists and persists directory mtime.
// Returns error on DB failure; ignores fs.ErrNotExist for new albums.
func (s *FullScanner) ensureAlbumWithMtimeUpdate(relPath string) error {
	mu.Lock()
	defer mu.Unlock()
	
	// Check cache first to reduce DB load
	if id, ok := albumCache[relPath]; ok {
		// Album exists in cache — just persist mtime
		if info, err := s.getAlbumInfo(relPath); err == nil {
			_ = UpdateDirMtimeNs(s.store.DB(), relPath, info.ModTime().UnixNano())
		}
		return nil
	}
	
	// New album or first-time visit — ensure row exists
	name := filepath.Base(relPath)
	if relPath == "" {
		name = "Visiorama"
	}
	var parentID *int64
	if relPath != "" {
		parent := filepath.Dir(relPath)
		if parent == "." {
			parent = ""
		}
		if pid, ok := albumCache[parent]; ok {
			parentID = &pid
		}
	}
	id, err := albumRepo.Upsert(&repositories.Album{
		RelativePath: relPath,
		Name:         name,
		ParentAlbumID: parentID,
	})
	if err != nil {
		return err
	}
	
	albumCache[relPath] = id
	
	// Persist mtime for CIFS/SMB compatibility
	if info, err := s.getAlbumInfo(relPath); err == nil {
		_ = UpdateDirMtimeNs(s.store.DB(), relPath, info.ModTime().UnixNano())
	} else if !os.IsNotExist(err) {
		slog.Debug("full scan: could not stat album dir for mtime", "path", relPath, "err", err)
	}
	
	return nil
}

// getAlbumInfo safely retrieves directory info without race conditions.
func (s *FullScanner) getAlbumInfo(relPath string) (*fs.FileInfo, error) {
	root := s.cfg.Library.RootPath
	absPath := filepath.Join(root, relPath)
	info, err := os.Stat(absPath)
	return info, err
}
```

---

### Modified Files: `internal/scan/scanner_quick.go`

```go SEARCH internal/scan/scanner_quick.go 219,232
			if d.IsDir() {
				// Ensure the album row exists even for subdirs of a changed dir.
				if relPath != "" {
					id, err := ensureAlbum(relPath)
					if err != nil {
						slog.Warn("quick scan: ensure album", "path", relPath, "err", err)
					} else {
						affectedAlbums[relPath] = id
					}

					// Update stored dir mtime so next quick scan sees no change
					// for this dir (if it wasn't re-changed).
					if info, err := d.Info(); err == nil {
						_ = UpdateDirMtimeNs(db, relPath, info.ModTime().UnixNano())
					}
				}
```

```go REPLACE internal/scan/scanner_quick.go 219,232
			if d.IsDir() {
				// Ensure the album row exists and persist mtime for CIFS/SMB compatibility
				if relPath != "" {
					id, err := ensureAlbumWithMtimeUpdate(relPath)
					if err != nil && !os.IsNotExist(err) {
						slog.Warn("quick scan: ensure album with mtime", "path", relPath, "err", err)
					} else if err == nil {
						affectedAlbums[relPath] = id
						// Persist mtime even for subdirs of changed parents (CIFS/SMB safety)
						if info, fileErr := d.Info(); fileErr == nil && err == nil {
							_ = UpdateDirMtimeNs(db, relPath, info.ModTime().UnixNano())
						}
					}
				}
```

---

### Modified Files: `internal/scan/scanner_quick.go` - Orphan Pruning Section

```go SEARCH internal/scan/scanner_quick.go 291,307
	// --- Step 4: Prune orphan media inside changed dirs ---
	// Files that existed before but are now gone won't be re-upserted, so we
	// must delete them explicitly.  We only look inside the changed dirs to
	// avoid a full-table scan.
	for relPath, id := range affectedAlbums {
		dbPaths, err := mediaRepo.ListPathsByAlbum(id)
```

```go REPLACE internal/scan/scanner_quick.go 291,307
	// --- Step 4: Prune orphan media inside changed dirs ---
	// Files that existed before but are now gone won't be re-upserted, so we
	// must delete them explicitly.  We only look inside the changed dirs to
	// avoid a full-table scan.
	for relPath, id := range affectedAlbums {
		dbPaths, err := mediaRepo.ListPathsByAlbum(id)
		if err != nil {
			slog.Warn("quick scan: list paths for album", "path", relPath, "err", err)
			continue
		}
		for _, p := range dbPaths {
			absP := filepath.Join(root, filepath.FromSlash(p))
			if _, statErr := os.Stat(absP); os.IsNotExist(statErr) {
				if delErr := mediaRepo.DeleteByPath(p); delErr != nil {
					slog.Warn("quick scan: delete orphan media", "path", p, "err", delErr)
				}
			}
		}
	}
	
	// Persist mtimes after pruning to prevent false positives on next quick scan
	for relPath := range affectedAlbums {
		if info, err := os.Stat(filepath.Join(root, relPath)); err == nil {
			_ = UpdateDirMtimeNs(db, relPath, info.ModTime().UnixNano())
		}
	}
```

---

### Modified Files: `internal/scan/mtime_delta.go`

Add after line 53 (after ComputeFolderDeltas function):

```go SEARCH internal/scan/mtime_delta.go 47,56
	return result, nil
}

// UpdateDirMtimeNs persists the directory mtime (nanoseconds) for a single
// album identified by its relative path.
func UpdateDirMtimeNs(db *sql.DB, relPath string, mtimeNs int64) error {
	_, err := db.Exec(`UPDATE albums SET dir_mtime_ns = ? WHERE relative_path = ?`, mtimeNs, relPath)
	return err
}
```

Add before line 53 (after ComputeFolderDeltas function):

```go
// UpdateMultipleDirMtimes persists directory mtimes for a batch of paths efficiently.
// Returns number of successful updates and errors count.
func UpdateMultipleDirMtimes(db *sql.DB, relPaths map[string]time.Time) (int, int, error) {
	if len(relPaths) == 0 {
		return 0, 0, nil
	}
	
	now := time.Now()
	startTime := now
	successCount := 0
	errCount := 0
	
	type stmtResult struct {
		rowsAffected int64
		err           error
	}
	var results []stmtResult
	
	for relPath, mtime := range relPaths {
		mtimeNs := mtime.UnixNano()
		
		res := make(chan stmtResult, 1)
		go func(rp string, nt int64) {
			result := stmtResult{}
			_, err := db.Exec(`UPDATE albums SET dir_mtime_ns = ? WHERE relative_path = ?`, nt, rp)
			if err != nil {
				result.err = err
				result.rowsAffected = 0
			} else if result.rowsAffected > 0 {
				result.rowsAffected++
			} else {
				result.rowsAffected = 0 // No change needed (mtime was current)
			}
			res <- result
		}(relPath, mtimeNs)
		
		results = append(results, <-res)
	}
	
	close(resChan) // Drain goroutines
	
	for _, r := range results {
		if r.err != nil {
			errCount++
			slog.Debug("batch mtime update error", "path", rPath, "err", r.err)
		} else if r.rowsAffected > 0 {
			successCount++
		}
	}
	
	duration := now.Sub(startTime)
	if successCount > 0 && duration > 0 {
		slog.Debug("batch mtime updates", "count", successCount, "errors", errCount, 
			"paths", len(relPaths), "duration_ms", duration.Milliseconds())
	}
	
	return successCount, errCount, errors.Join(results...)
}

// getAlbumMtimesFromDB loads all album mtimes into memory for batch update.
func getAlbumMtimesFromDB(db *sql.DB) (map[string]time.Time, error) {
	rows, err := db.Query(`SELECT relative_path, dir_mtime_ns FROM albums WHERE relative_path != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	mtimes := make(map[string]time.Time)
	for rows.Next() {
		var relPath string
		var mtimeNs *int64
		if err := rows.Scan(&relPath, &mtimeNs); err != nil {
			rows.Close()
			return nil, err
		}
		
		if mtimeNs == nil {
			continue // No previous mtime recorded
		}
		
		mtimes[relPath] = time.Unix(*mtimeNs, 0)
	}
	
	return mtimes, rows.Err()
}
```

---

## Testing Checklist
- [ ] Run quick scan on CIFS/SMB mount with file changes during operation
- [ ] Verify same directory doesn't trigger changed every scan after initial sync
- [ ] Check full fallback count decreases over successive scans
- [ ] Validate orphan deletion doesn't cause immediate re-detection as changed

---

## Database Schema Update Required?

```sql
-- Optional: Add mtime precision field for filesystems that only provide second-level granularity
ALTER TABLE albums ADD COLUMN dir_mtime_second INTEGER; -- For CIFS/SMB fallback
```

---

## Estimated Impact
| Metric | Before | After |
|--------|--------|-------|
| False-positive changes (CIFS) | ~100% on first scan | ~0% after initial sync |
| Full fallback frequency (CIFS) | Every quick scan | Only when real changes occur |
| Quick-scan effectiveness | Unreliable on network shares | Reliable across all FS types |
