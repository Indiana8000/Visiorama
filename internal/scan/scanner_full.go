package scan

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

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

func (s *FullScanner) Run(ctx context.Context, scanID string) (*Stats, error) {
	InitExtensions(s.cfg.Filtering.AllowedImageExtensions, s.cfg.Filtering.AllowedVideoExtensions)

	albumRepo := repositories.NewAlbumsRepo(s.store.DB())
	mediaRepo := repositories.NewMediaRepo(s.store.DB())
	scanRepo := repositories.NewScanRepo(s.store.DB())

	stats := &Stats{}
	excludeSet := buildExcludeSet(s.cfg.Filtering.ExcludePatterns)

	// seenMediaPaths collects every media relative path found on disk this run.
	seenMediaPaths := map[string]bool{}
	// seenAlbumPaths collects every album relative path found on disk this run.
	seenAlbumPaths := map[string]bool{"": true} // root always present

	// albumID cache: relativePath → id
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
			RelativePath: relPath,
			Name:         name,
			ParentAlbumID: parentID,
		})
		if err != nil {
			return 0, err
		}
		albumCache[relPath] = id
		return id, nil
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
					slog.Warn("upsert media", "path", item.relPath, "err", err)
					continue
				}
				stats.Indexed.Add(1)
			}
		}()
	}

	root := s.cfg.Library.RootPath
	slog.Info("full scan: starting walk", "root", root)
	walkErr := filepath.WalkDir(root, func(absPath string, d fs.DirEntry, err error) error {
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
			seenAlbumPaths[relPath] = true
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

		seenMediaPaths[relPath] = true

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

	slog.Info("full scan: walk complete",
		"scanned", stats.Scanned.Load(),
		"indexed", stats.Indexed.Load(),
		"skipped", stats.Skipped.Load(),
		"errors", stats.ErrCount.Load(),
		"seenAlbums", len(seenAlbumPaths),
		"seenMedia", len(seenMediaPaths),
		"walkErr", walkErr,
	)

	if walkErr != nil && walkErr != context.Canceled {
		return stats, walkErr
	}

	// Only clean orphans when walk actually found items — guards against a
	// misconfigured rootPath or a failed walk wiping the entire index.
	if len(seenMediaPaths) > 0 || len(seenAlbumPaths) > 1 {
		if allMediaPaths, err := mediaRepo.ListAllPaths(); err == nil {
			for _, p := range allMediaPaths {
				if !seenMediaPaths[p] {
					if err := mediaRepo.DeleteByPath(p); err != nil {
						slog.Warn("delete orphan media", "path", p, "err", err)
					}
				}
			}
		}

		if allAlbumPaths, err := albumRepo.ListAllPaths(); err == nil {
			sortByDepthDesc(allAlbumPaths)
			for _, p := range allAlbumPaths {
				if !seenAlbumPaths[p] {
					if err := albumRepo.DeleteByPath(p); err != nil {
						slog.Warn("delete orphan album", "path", p, "err", err)
					}
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

func (s *FullScanner) recomputeCounts(albumRepo *repositories.AlbumsRepo, mediaRepo *repositories.MediaRepo, cache map[string]int64) error {
	// Process deepest paths first so parent recursive counts include children.
	paths := make([]string, 0, len(cache))
	for p := range cache {
		paths = append(paths, p)
	}
	sortByDepthDesc(paths)

	recursiveCounts := map[int64]int{}

	for _, relPath := range paths {
		id := cache[relPath]
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
			if strings.Count(paths[i], "/") < strings.Count(paths[j], "/") {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}
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
