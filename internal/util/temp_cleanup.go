package util

import (
	"log/slog"
	"os"
	"path/filepath"
)

// CleanupTempFiles removes files in the system temporary directory that match the provided glob patterns.
func CleanupTempFiles(patterns ...string) {
	tempDir := os.TempDir()
	for _, pattern := range patterns {
		// Join tempDir with the pattern to create a full glob pattern
		fullPattern := filepath.Join(tempDir, pattern)
		matches, err := filepath.Glob(fullPattern)
		if err != nil {
			slog.Error("failed to glob temp files", "pattern", fullPattern, "err", err)
			continue
		}

		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				// We log a warning instead of an error because some files might be in use or already gone
				slog.Warn("failed to remove temp file", "path", match, "err", err)
			} else {
				slog.Debug("removed old temp file", "path", match)
			}
		}
	}
}
