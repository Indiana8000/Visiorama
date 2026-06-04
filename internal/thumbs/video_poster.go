package thumbs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// GenerateVideoPoster extracts the first frame of a video as JPEG via ffmpeg.
// Falls back gracefully if ffmpeg is not installed.
func GenerateVideoPoster(srcPath, cacheDir string, size int) (string, error) {
	cachePath := CachePath(cacheDir, srcPath, size)

	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return "", fmt.Errorf("mkdir cache: %w", err)
	}

	// Extract frame at 0s, fit within size×size, maintain aspect ratio.
	filter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", size, size)
	cmd := exec.Command("ffmpeg",
		"-y",
		"-ss", "0",
		"-i", srcPath,
		"-vframes", "1",
		"-vf", filter,
		"-q:v", "3",
		cachePath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg: %w — %s", err, string(out))
	}
	return cachePath, nil
}

// FFmpegAvailable returns true if ffmpeg is found in PATH.
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// FFmpegPath returns the resolved path to the ffmpeg binary, or empty string if not found.
func FFmpegPath() string {
	p, _ := exec.LookPath("ffmpeg")
	return p
}
