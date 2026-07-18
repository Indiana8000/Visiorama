package scan

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

// OrphanScanner checks every media path in the DB against disk and removes
// entries whose files no longer exist.  It does not walk the filesystem for
// new files — use QuickScanner or FullScanner for that.
type OrphanScanner struct {
	cfg   *app.Config
	store *index.Store
}

func NewOrphanScanner(cfg *app.Config, store *index.Store) *OrphanScanner {
	return &OrphanScanner{cfg: cfg, store: store}
}

func (s *OrphanScanner) Run(ctx context.Context, scanID string) (*Stats, error) {
	db := s.store.DB()
	root := s.cfg.Library.RootPath
	mediaRepo := repositories.NewMediaRepo(db)
	albumRepo := repositories.NewAlbumsRepo(db)
	scanRepo := repositories.NewScanRepo(db)

	stats := &Stats{}

	allPaths, err := mediaRepo.ListAllPaths()
	if err != nil {
		return nil, err
	}

	affectedAlbums := map[string]int64{}

	for _, p := range allPaths {
		if ctx.Err() != nil {
			break
		}
		stats.Scanned.Add(1)

		absP := filepath.Join(root, filepath.FromSlash(p))
		if _, statErr := os.Stat(absP); !os.IsNotExist(statErr) {
			continue
		}

		if delErr := mediaRepo.DeleteByPath(p); delErr != nil {
			stats.ErrCount.Add(1)
			_ = scanRepo.AddError(scanID, p, delErr.Error())
			slog.Warn("orphan scan: delete failed", "path", p, "err", delErr)
			continue
		}

		stats.Indexed.Add(1)
		slog.Info("orphan scan: removed deleted media", "path", p)

		albumDir := filepath.ToSlash(filepath.Dir(p))
		if albumDir == "." {
			albumDir = ""
		}
		if _, already := affectedAlbums[albumDir]; !already {
			if a, aErr := albumRepo.GetByPath(albumDir); aErr == nil && a != nil {
				affectedAlbums[albumDir] = a.ID
			}
		}
	}

	// Recompute counts for affected albums + ancestors.
	closure := map[string]int64{}
	for relPath, id := range affectedAlbums {
		closure[relPath] = id
	}
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
			a, aErr := albumRepo.GetByPath(parent)
			if aErr != nil || a == nil {
				break
			}
			closure[parent] = a.ID
			p = parent
		}
	}

	closurePaths := make([]string, 0, len(closure))
	for p := range closure {
		closurePaths = append(closurePaths, p)
	}
	sortByDepthDesc(closurePaths)

	recursiveCounts := map[int64]int{}
	for _, relPath := range closurePaths {
		id := closure[relPath]
		direct, cErr := mediaRepo.CountByAlbum(id)
		if cErr != nil {
			continue
		}
		children, cErr := albumRepo.ListChildren(id)
		if cErr != nil {
			continue
		}
		recursive := direct
		for _, child := range children {
			if updated, ok := recursiveCounts[child.ID]; ok {
				recursive += updated
			} else {
				recursive += child.MediaCountRecursive
			}
		}
		recursiveCounts[id] = recursive
		_ = albumRepo.UpdateCounts(id, direct, recursive, len(children))
	}

	if rootAlbum, rErr := albumRepo.GetRoot(); rErr == nil && rootAlbum != nil {
		rootChildren, _ := albumRepo.ListChildren(rootAlbum.ID)
		rootDirect, _ := mediaRepo.CountByAlbum(rootAlbum.ID)
		rootRecursive := rootDirect
		for _, child := range rootChildren {
			if updated, ok := recursiveCounts[child.ID]; ok {
				rootRecursive += updated
			} else {
				rootRecursive += child.MediaCountRecursive
			}
		}
		_ = albumRepo.UpdateCounts(rootAlbum.ID, rootDirect, rootRecursive, len(rootChildren))
	}

	// Remove albums whose directories no longer exist on disk (skip root).
	allAlbums, err := albumRepo.ListAll()
	if err == nil {
		for _, a := range allAlbums {
			if a.RelativePath == "" {
				continue
			}
			absDir := filepath.Join(root, filepath.FromSlash(a.RelativePath))
			if _, statErr := os.Stat(absDir); os.IsNotExist(statErr) {
				if delErr := albumRepo.DeleteByID(a.ID); delErr != nil {
					slog.Warn("orphan scan: delete album failed", "path", a.RelativePath, "err", delErr)
				} else {
					slog.Info("orphan scan: removed deleted album", "path", a.RelativePath)
					stats.Indexed.Add(1)
				}
			}
		}
	}

	return stats, nil
}
