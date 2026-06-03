package thumbs

import (
	"crypto/sha1"
	"fmt"
	"path/filepath"
)

// CachePath returns the deterministic JPEG path for a given source file + size.
func CachePath(cacheDir, srcPath string, size int) string {
	h := sha1.Sum([]byte(srcPath))
	hash := fmt.Sprintf("%x", h)
	// two-level dir to avoid huge flat directories
	return filepath.Join(cacheDir, fmt.Sprintf("%d", size), hash[:2], hash+".jpg")
}
