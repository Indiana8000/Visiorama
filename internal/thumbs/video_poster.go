package thumbs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// GenerateVideoPoster extracts the first frame of a video as JPEG via ffmpeg,
// scaled and cropped to exactly width×height.
func GenerateVideoPoster(srcPath, cacheDir string, width, height int) (string, error) {
	cachePath := CachePath(cacheDir, srcPath, width, height)

	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return "", fmt.Errorf("mkdir cache: %w", err)
	}

	// Scale to fill width×height, then center-crop to exact dimensions.
	filter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d",
		width, height, width, height,
	)
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

// ImageMagickAvailable returns true if the ImageMagick "magick" binary is found in PATH.
func ImageMagickAvailable() bool {
	_, err := exec.LookPath("magick")
	return err == nil
}

// ImageMagickPath returns the resolved path to the "magick" binary, or empty string.
func ImageMagickPath() string {
	p, _ := exec.LookPath("magick")
	return p
}
