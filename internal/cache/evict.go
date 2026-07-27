// Package cache provides disk-budget enforcement shared by the thumbnail and
// transcode caches.
package cache

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SafeDir returns an error if dir is empty, relative, or suspiciously close
// to the filesystem root (e.g. "/" or "C:\"). Mirrors the guard used by the
// manual thumbnail reset endpoint so eviction never walks an unintended path.
func SafeDir(dir string) error {
	abs := filepath.Clean(dir)
	if !filepath.IsAbs(abs) {
		return fmt.Errorf("cache dir must be absolute, got %q", dir)
	}
	parent := filepath.Dir(abs)
	if parent == abs || strings.Count(filepath.ToSlash(abs), "/") < 2 {
		return fmt.Errorf("cache dir %q is too close to filesystem root", dir)
	}
	return nil
}

// EvictToBudget walks dir and deletes the oldest-modified regular files
// first until total size is at or under maxBytes. maxBytes <= 0 disables
// enforcement (no-op). Returns the number of files removed and bytes freed.
func EvictToBudget(dir string, maxBytes int64) (removed int, freed int64, err error) {
	if maxBytes <= 0 {
		return 0, 0, nil
	}
	if err := SafeDir(dir); err != nil {
		return 0, 0, err
	}

	type fileInfo struct {
		path    string
		size    int64
		modTime int64
	}
	var files []fileInfo
	var total int64

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files = append(files, fileInfo{path: path, size: info.Size(), modTime: info.ModTime().UnixNano()})
		total += info.Size()
		return nil
	})
	if walkErr != nil {
		return 0, 0, fmt.Errorf("walk %s: %w", dir, walkErr)
	}

	if total <= maxBytes {
		return 0, 0, nil
	}

	sort.Slice(files, func(i, j int) bool { return files[i].modTime < files[j].modTime })

	for _, f := range files {
		if total <= maxBytes {
			break
		}
		if err := os.Remove(f.path); err != nil {
			continue
		}
		total -= f.size
		freed += f.size
		removed++
	}

	return removed, freed, nil
}
