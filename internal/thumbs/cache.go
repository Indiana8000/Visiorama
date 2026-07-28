package thumbs

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
)

// CachePath returns the deterministic JPEG path for a given source file + dimensions.
func CachePath(cacheDir, srcPath string, width, height int) string {
	h := sha1.Sum([]byte(srcPath))
	hash := fmt.Sprintf("%x", h)
	// two-level dir to avoid huge flat directories
	return filepath.Join(cacheDir, fmt.Sprintf("%dx%d", width, height), hash[:2], hash+".jpg")
}

// DeleteCached removes every cached thumbnail JPEG for the given absolute source
// path across all configured widths. Image thumbnails and video posters share the
// same CachePath scheme, so one call covers both media types. Call this whenever
// a media row is deleted, or its cache file becomes an orphan on disk forever.
func DeleteCached(cacheDir, srcAbsPath string, widths []int, aspectW, aspectH int) (removed int) {
	for _, w := range widths {
		h := w
		if aspectW > 0 && aspectH > 0 {
			h = w * aspectH / aspectW
		}
		if err := os.Remove(CachePath(cacheDir, srcAbsPath, w, h)); err == nil {
			removed++
		}
	}
	return removed
}
