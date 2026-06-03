package util

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafeJoin joins root and relPath, returning an error if the result would escape root.
func SafeJoin(root, relPath string) (string, error) {
	abs := filepath.Join(root, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path traversal detected: %q", relPath)
	}
	return abs, nil
}
