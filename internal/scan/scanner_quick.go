package scan

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/USERNAME/visiorama/internal/app"
	"github.com/USERNAME/visiorama/internal/index"
	"github.com/USERNAME/visiorama/internal/index/repositories"
)

// QuickScanner uses mtime deltas to find changed folders and re-scans only
// those, falling back to FullScanner when uncertainty rules apply (ADR-003).
type QuickScanner struct {
	cfg   *app.Config
	store *index.Store
}

func NewQuickScanner(cfg *app.Config, store *index.Store) *QuickScanner {
	return &QuickScanner{cfg: cfg, store: store}
}

// Run executes the quick scan for the given scanID.
// Returns (stats, fallbackOccurred, error).
func (s *QuickScanner) Run(ctx context.Context, scanID string) (*Stats, bool, error) {
	InitExtensions(s.cfg.Filtering.AllowedImageExtensions, s.cfg.Filtering.AllowedVideoExtensions)

	db := s.store.DB()
	excludeSet := buildExcludeSet(s.cfg.Filtering.ExcludePatterns)
	root := s.cfg.Library.RootPath

	// --- Step 1: Compute folder deltas ---
	delta, err := ComputeFolderDeltas(db, root, excludeSet)
	if err != nil {
		return nil, false, fmt.Errorf("compute folder deltas: %w", err)
	}

	// --- Step 2: Check uncertainty rules → fall back if needed ---
	if delta.DBEmpty {
		slog.Info("quick scan: DB empty, falling back to full scan", "scanID", scanID)
		stats, err := NewFullScanner(s.cfg, s.store).Run(ctx, scanID)
		return stats, true, err
	}
	if len(delta.DeletedDirs) > 0 {
		slog.Info("quick scan: deleted dirs detected, falling back to full scan",
			"scanID", scanID, "deletedDirs", delta.DeletedDirs)
		stats, err := NewFullScanner(s.cfg, s.store).Run(ctx, scanID)
		return stats, true, err
	}

	// --- Step 3: Nothing changed — nothing to do ---
	if len(delta.ChangedDirs) == 0 {
		slog.Info("quick scan: no changed dirs, nothing to do", "scanID", scanID)
		return &Stats{}, false, nil
	}

	slog.Info("quick scan: re-scanning changed dirs", "scanID", scanID, "count", len(delta.ChangedDirs))

	albumRepo := repositories.NewAlbumsRepo(db)
	mediaRepo := repositories.NewMediaRepo(db)
	scanRepo := repositories.NewScanRepo(db)

	stats := &Stats{}

	// albumID cache: relativePath → id (same pattern as FullScanner)
	albumCache := map[string]int64{}
	var mu sync.Mutex

	ensureAlbum := func(relPath string) (int64, error) {
		mu.Lock()
		defer mu.Unlock()
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
			if pid, ok := albumCache[parent]; ok {
				parentID = &pid
			}
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
		return id, nil
	}

	// Pre-seed album cache with root so parent lookups work for top-level dirs.
	if _, err := ensureAlbum(""); err != nil {
		return nil, false, fmt.Errorf("ensure root album: %w", err)
	}

	// Build a set of changed dirs for O(1) lookup.
	changedSet := make(map[string]bool, len(delta.ChangedDirs))
	for _, d := range delta.ChangedDirs {
		changedSet[d] = true
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
	if workers < 1 {
		workers = 4
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
					slog.Warn("quick scan: upsert media", "path", item.relPath, "err", err)
					continue
				}
				stats.Indexed.Add(1)
			}
		}()
	}

	// Walk only inside changed directories — skip unchanged dirs entirely.
	// We still call WalkDir per changed dir so we get subdirectory structure.
	affectedAlbums := map[string]int64{} // relPath → albumID for count recompute

	for _, changedRelPath := range delta.ChangedDirs {
		if ctx.Err() != nil {
			break
		}

		changedAbsPath := filepath.Join(root, filepath.FromSlash(changedRelPath))

		walkErr := filepath.WalkDir(changedAbsPath, func(absPath string, d fs.DirEntry, err error) error {
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
				return nil
			}

			stats.Scanned.Add(1)
			mtype, ext, mime := Classify(absPath, s.cfg.Filtering.EnableMimeSniff)
			if mtype == MediaTypeUnknown {
				stats.Skipped.Add(1)
				return nil
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

		if walkErr != nil && walkErr != context.Canceled {
			slog.Warn("quick scan: walk error", "dir", changedRelPath, "err", walkErr)
		}
	}

	close(jobs)
	wg.Wait()

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
				if err := mediaRepo.DeleteByPath(p); err != nil {
					slog.Warn("quick scan: delete orphan media", "path", p, "err", err)
				}
			}
		}
	}

	// --- Step 5: Recompute counts for affected albums + ancestors ---
	// Build the full ancestor closure so parent counts stay correct.
	// Process deepest paths first (same strategy as FullScanner.recomputeCounts).
	closure := map[string]int64{}
	for relPath, id := range affectedAlbums {
		closure[relPath] = id
	}
	// Walk up the tree for each affected album.
	for relPath := range affectedAlbums {
		p := relPath
		for p != "" {
			parent := filepath.ToSlash(filepath.Dir(p))
			if parent == "." {
				parent = ""
			}
			if _, seen := closure[parent]; seen {
				break
			}
			a, err := albumRepo.GetByPath(parent)
			if err != nil || a == nil {
				break
			}
			closure[parent] = a.ID
			p = parent
		}
	}

	// Sort deepest-first.
	closurePaths := make([]string, 0, len(closure))
	for p := range closure {
		closurePaths = append(closurePaths, p)
	}
	sortByDepthDesc(closurePaths)

	recursiveCounts := map[int64]int{}
	for _, relPath := range closurePaths {
		id := closure[relPath]
		direct, err := mediaRepo.CountByAlbum(id)
		if err != nil {
			slog.Warn("quick scan: count media", "path", relPath, "err", err)
			continue
		}
		children, err := albumRepo.ListChildren(id)
		if err != nil {
			slog.Warn("quick scan: list children", "path", relPath, "err", err)
			continue
		}
		recursive := direct
		for _, child := range children {
			recursive += recursiveCounts[child.ID]
		}
		recursiveCounts[id] = recursive
		if err := albumRepo.UpdateCounts(id, direct, recursive, len(children)); err != nil {
			slog.Warn("quick scan: update counts", "path", relPath, "err", err)
		}
	}

	// Always recompute root (not in affectedAlbums).
	if rootAlbum, err := albumRepo.GetRoot(); err == nil && rootAlbum != nil {
		rootChildren, _ := albumRepo.ListChildren(rootAlbum.ID)
		rootDirect, _ := mediaRepo.CountByAlbum(rootAlbum.ID)
		rootRecursive := rootDirect
		for _, child := range rootChildren {
			rootRecursive += recursiveCounts[child.ID]
		}
		_ = albumRepo.UpdateCounts(rootAlbum.ID, rootDirect, rootRecursive, len(rootChildren))
	}

	return stats, false, nil
}
