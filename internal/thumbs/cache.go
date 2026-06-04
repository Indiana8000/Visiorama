package thumbs

import (
	"crypto/sha1"
	"fmt"
	"path/filepath"
)

// CachePath returns the deterministic JPEG path for a given source file + dimensions.
func CachePath(cacheDir, srcPath string, width, height int) string {
	h := sha1.Sum([]byte(srcPath))
	hash := fmt.Sprintf("%x", h)
	// two-level dir to avoid huge flat directories
	return filepath.Join(cacheDir, fmt.Sprintf("%dx%d", width, height), hash[:2], hash+".jpg")
}
