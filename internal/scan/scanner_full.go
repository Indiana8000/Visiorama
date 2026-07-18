package scan

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

type Stats struct {
	Scanned  atomic.Int64
	Indexed  atomic.Int64
	Skipped  atomic.Int64
	ErrCount atomic.Int64
}

type FullScanner struct {
	cfg   *app.Config
	store *index.Store
}

func NewFullScanner(cfg *app.Config, store *index.Store) *FullScanner {
	return &FullScanner{cfg: cfg, store: store}
}

// ProgressFunc is called periodically during a full scan with a snapshot of current counters.
type ProgressFunc func(scanned, indexed, skipped, errors int64)

func (s *FullScanner) Run(ctx context.Context, scanID string) (*Stats, error) {
	return s.RunWithProgress(ctx, scanID, "", nil)
}

func (s *FullScanner) RunWithProgress(ctx context.Context, scanID string, albumPath string, onProgress ProgressFunc) (*Stats, error) {
	InitExtensions(s.cfg.Filtering.AllowedImageExtensions, s.cfg.Filtering.AllowedVideoExtensions)
	if albumPath != "" {
		slog.Debug("full scan: subtree mode", "scanID", scanID, "albumPath", albumPath)
	} else {
		slog.Debug("full scan: full library mode", "scanID", scanID)
	}

	db := s.store.DB()
	albumRepo := repositories.NewAlbumsRepo(db)
	mediaRepo := repositories.NewMediaRepo(db)
	scanRepo := repositories.NewScanRepo(db)

	// Use SQLite temp tables to track seen paths — avoids holding all paths in RAM.
	for _, ddl := range []string{
		`CREATE TEMPORARY TABLE IF NOT EXISTS _seen_media  (path TEXT PRIMARY KEY)`,
		`CREATE TEMPORARY TABLE IF NOT EXISTS _seen_albums (path TEXT PRIMARY KEY)`,
		`DELETE FROM _seen_media`,
		`DELETE FROM _seen_albums`,
		`INSERT OR IGNORE INTO _seen_albums VALUES ('')`, // root always present
	} {
		if _, err := db.Exec(ddl); err != nil {
			return nil, fmt.Errorf("setup seen tables: %w", err)
		}
	}

	// When scanning a subtree, pre-seed seen tables with everything outside it
	// so the orphan-cleanup step does not delete albums/media outside the scope.
	if albumPath != "" {
		var nAlbums, nMedia int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM albums WHERE relative_path NOT LIKE ? AND relative_path != ?`,
			albumPath+"/%", albumPath,
		).Scan(&nAlbums); err == nil {
			slog.Debug("full scan: pre-seeding out-of-scope albums", "count", nAlbums, "albumPath", albumPath)
		}
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO _seen_albums SELECT relative_path FROM albums WHERE relative_path NOT LIKE ? AND relative_path != ?`,
			albumPath+"/%", albumPath,
		); err != nil {
			return nil, fmt.Errorf("pre-seed seen_albums: %w", err)
		}
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM media WHERE relative_path NOT LIKE ?`,
			albumPath+"/%",
		).Scan(&nMedia); err == nil {
			slog.Debug("full scan: pre-seeding out-of-scope media", "count", nMedia, "albumPath", albumPath)
		}
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO _seen_media SELECT relative_path FROM media WHERE relative_path NOT LIKE ?`,
			albumPath+"/%",
		); err != nil {
			return nil, fmt.Errorf("pre-seed seen_media: %w", err)
		}
	}

	stats := &Stats{}
	excludeSet := buildExcludeSet(s.cfg.Filtering.ExcludePatterns)

	// albumID cache: relativePath → id
	albumCache := map[string]int64{}
	var mu sync.Mutex

	// ensureAlbumLocked upserts an album and all missing ancestors. Must be called with mu held.
	var ensureAlbumLocked func(relPath string) (int64, error)
	ensureAlbumLocked = func(relPath string) (int64, error) {
		if id, ok := albumCache[relPath]; ok {
			return id, nil
		}
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
			pid, err := ensureAlbumLocked(parent)
			if err != nil {
				return 0, err
			}
			parentID = &pid
		}
		id, err := albumRepo.Upsert(&repositories.Album{
			RelativePath:  relPath,
			Name:          name,
			ParentAlbumID: parentID,
		})
		if err != nil {
			return 0, err
		}
		albumCache[relPath] = id
		if _, err := db.Exec(`INSERT OR IGNORE INTO _seen_albums VALUES (?)`, relPath); err != nil {
			slog.Warn("insert seen_album", "path", relPath, "err", err)
		}
		return id, nil
	}

	ensureAlbum := func(relPath string) (int64, error) {
		mu.Lock()
		defer mu.Unlock()
		return ensureAlbumLocked(relPath)
	}

	// Ensure root album exists
	if _, err := ensureAlbum(""); err != nil {
		return nil, fmt.Errorf("ensure root album: %w", err)
	}

	type workItem struct {
		absPath  string
		relPath  string
		filename string
		ext      string
		mime     string
		mtype    MediaType
		albumID  int64
	}

	jobs := make(chan workItem, 256)
	workers := s.cfg.Scan.MaxWorkers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	const progressEveryN = 100
	var lastReported atomic.Int64

	flush := func() {
		sc := stats.Scanned.Load()
		idx := stats.Indexed.Load()
		sk := stats.Skipped.Load()
		er := stats.ErrCount.Load()
		slog.Info("full scan: progress",
			"scanned", sc, "indexed", idx, "skipped", sk, "errors", er)
		if onProgress != nil {
			onProgress(sc, idx, sk, er)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if ctx.Err() != nil {
					return
				}
				var m *repositories.Media
				switch item.mtype {
				case MediaTypeImage:
					m, _ = ExtractImageMeta(item.absPath, item.relPath, item.filename,
						item.ext, item.mime, item.albumID, s.cfg.Limits.LargeMediaWarningBytes)
				case MediaTypeVideo:
					m, _ = ExtractVideoMeta(item.absPath, item.relPath, item.filename,
						item.ext, item.mime, item.albumID, s.cfg.Limits.LargeMediaWarningBytes)
				}
				if m == nil {
					stats.Skipped.Add(1)
					continue
				}
				if _, err := mediaRepo.Upsert(m); err != nil {
					stats.ErrCount.Add(1)
					_ = scanRepo.AddError(scanID, item.relPath, err.Error())
					slog.Warn("full scan: upsert media", "path", item.relPath, "err", err)
					continue
				}
				newIdx := stats.Indexed.Add(1)
				// count-based flush: every progressEveryN indexed files
				prev := lastReported.Load()
				if newIdx-prev >= progressEveryN && lastReported.CompareAndSwap(prev, newIdx) {
					flush()
				}
			}
		}()
	}

	// Time-based flush: every 3 seconds, independent of count
	tickCtx, cancelTick := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				flush()
			case <-tickCtx.Done():
				return
			}
		}
	}()

	root := s.cfg.Library.RootPath
	walkRoot := root
	if albumPath != "" {
		walkRoot = filepath.Join(root, filepath.FromSlash(albumPath))
	}
	slog.Info("full scan: starting walk", "root", walkRoot)
	walkErr := filepath.WalkDir(walkRoot, func(absPath string, d fs.DirEntry, err error) error {
		if err != nil {
			stats.ErrCount.Add(1)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		name := d.Name()
		if isExcluded(name, excludeSet) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(root, absPath)
		relPath = filepath.ToSlash(relPath)
		if relPath == "." {
			relPath = ""
		}

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
			return nil
		}

		stats.Scanned.Add(1)
		mtype, ext, mime := Classify(absPath, s.cfg.Filtering.EnableMimeSniff)
		slog.Debug("full scan: file", "path", relPath, "mtype", mtype, "ext", ext)
		if mtype == MediaTypeUnknown {
			slog.Info("full scan: skipped (unknown type)", "path", relPath, "ext", ext)
			stats.Skipped.Add(1)
			return nil
		}

		if _, err := db.Exec(`INSERT OR IGNORE INTO _seen_media VALUES (?)`, relPath); err != nil {
			slog.Warn("insert seen_media", "path", relPath, "err", err)
		}

		albumRelPath := filepath.ToSlash(filepath.Dir(relPath))
		if albumRelPath == "." {
			albumRelPath = ""
		}
		albumID, err := ensureAlbum(albumRelPath)
		if err != nil {
			stats.ErrCount.Add(1)
			return nil
		}

		jobs <- workItem{
			absPath:  absPath,
			relPath:  relPath,
			filename: name,
			ext:      ext,
			mime:     mime,
			mtype:    mtype,
			albumID:  albumID,
		}
		return nil
	})

	close(jobs)
	wg.Wait()
	cancelTick()
	flush() // final flush after all workers done

	var seenMediaCount, seenAlbumCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM _seen_media`).Scan(&seenMediaCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM _seen_albums`).Scan(&seenAlbumCount)

	slog.Info("full scan: walk complete",
		"scanned", stats.Scanned.Load(),
		"indexed", stats.Indexed.Load(),
		"skipped", stats.Skipped.Load(),
		"errors", stats.ErrCount.Load(),
		"seenAlbums", seenAlbumCount,
		"seenMedia", seenMediaCount,
		"walkErr", walkErr,
	)

	if walkErr != nil && walkErr != context.Canceled {
		return stats, walkErr
	}

	// Only clean orphans when walk actually found items — guards against a
	// misconfigured rootPath or a failed walk wiping the entire index.
	if seenMediaCount > 0 || seenAlbumCount > 1 {
		// Delete orphan media: paths in DB but not seen on disk this run.
		if orphanPaths, err := mediaRepo.ListOrphanPaths(db); err == nil {
			slog.Debug("full scan: orphan media candidates", "count", len(orphanPaths))
			for _, p := range orphanPaths {
				slog.Debug("full scan: deleting orphan media", "path", p)
				if err := mediaRepo.DeleteByPath(p); err != nil {
					slog.Warn("delete orphan media", "path", p, "err", err)
				}
			}
		}

		// Delete orphan albums deepest-first so parent counts are correct.
		if orphanPaths, err := albumRepo.ListOrphanPaths(db); err == nil {
			sortByDepthDesc(orphanPaths)
			slog.Debug("full scan: orphan album candidates", "count", len(orphanPaths))
			for _, p := range orphanPaths {
				slog.Debug("full scan: deleting orphan album", "path", p)
				if err := albumRepo.DeleteByPath(p); err != nil {
					slog.Warn("delete orphan album", "path", p, "err", err)
				}
			}
		}
	}

	// Recompute counts for all albums
	if err := s.recomputeCounts(albumRepo, mediaRepo, albumCache); err != nil {
		slog.Warn("recompute counts", "err", err)
	}

	return stats, nil
}

func (s *FullScanner) recomputeCounts(albumRepo *repositories.AlbumsRepo, mediaRepo *repositories.MediaRepo, _ map[string]int64) error {
	// Load current DB state — avoids stale walk-cache IDs after orphan deletion.
	dbCache, err := albumRepo.ListAllPathIDs()
	if err != nil {
		return err
	}

	// Process deepest paths first so parent recursive counts include children.
	paths := make([]string, 0, len(dbCache))
	for p := range dbCache {
		paths = append(paths, p)
	}
	sortByDepthDesc(paths)

	recursiveCounts := map[int64]int{}

	for _, relPath := range paths {
		id := dbCache[relPath]
		direct, err := mediaRepo.CountByAlbum(id)
		if err != nil {
			return err
		}
		children, err := albumRepo.ListChildren(id)
		if err != nil {
			return err
		}

		recursive := direct
		for _, child := range children {
			recursive += recursiveCounts[child.ID]
		}
		recursiveCounts[id] = recursive

		if err := albumRepo.UpdateCounts(id, direct, recursive, len(children)); err != nil {
			return err
		}
	}
	return nil
}

func buildExcludeSet(patterns []string) map[string]bool {
	m := map[string]bool{}
	for _, p := range patterns {
		m[p] = true
	}
	return m
}

func sortByDepthDesc(paths []string) {
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if pathDepth(paths[i]) < pathDepth(paths[j]) {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}
}

// pathDepth returns the sort depth of a path. Root "" gets -1 so it always
// sorts after every other path in a deepest-first ordering.
func pathDepth(p string) int {
	if p == "" {
		return -1
	}
	return strings.Count(p, "/")
}

func isExcluded(name string, excludeSet map[string]bool) bool {
	if excludeSet[name] {
		return true
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return false
}
