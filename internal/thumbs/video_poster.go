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

	// Extract frame at 0s, scale longest edge to `size`, single frame output
	filter := fmt.Sprintf("scale='if(gt(iw,ih),%d,-2)':'if(gt(iw,ih),-2,%d)'", size, size)
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
