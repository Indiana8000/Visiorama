package thumbs

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/disintegration/imaging"
)

// Generate resizes the image at srcPath to the given max dimension (longest edge)
// and writes a JPEG to the cache path. Returns the cache path.
// Falls back to ffmpeg for formats unsupported by Go's image decoders (e.g. HEIC).
func Generate(srcPath, cacheDir string, size int) (string, error) {
	cachePath := CachePath(cacheDir, srcPath, size)

	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil // cache hit
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return "", fmt.Errorf("mkdir cache: %w", err)
	}

	src, err := imaging.Open(srcPath, imaging.AutoOrientation(true))
	if err != nil {
		// Go decoder failed — try ffmpeg (handles HEIC, AVIF, TIFF, etc.)
		if FFmpegAvailable() {
			return generateViaFFmpeg(srcPath, cachePath, size)
		}
		return "", fmt.Errorf("open image: %w (ffmpeg not available as fallback)", err)
	}

	thumb := resizeFit(src, size)

	if err := imaging.Save(thumb, cachePath, imaging.JPEGQuality(82)); err != nil {
		return "", fmt.Errorf("save thumb: %w", err)
	}
	return cachePath, nil
}

// generateViaFFmpeg uses ffmpeg to decode and scale the image to a JPEG thumbnail.
func generateViaFFmpeg(srcPath, cachePath string, size int) (string, error) {
	filter := fmt.Sprintf("scale='if(gt(iw,ih),%d,-2)':'if(gt(iw,ih),-2,%d)',format=yuvj420p", size, size)
	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", srcPath,
		"-vframes", "1",
		"-vf", filter,
		"-q:v", "3",
		cachePath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg fallback: %w — %s", err, string(out))
	}
	return cachePath, nil
}

// resizeFit scales img so the longest edge == size, preserving aspect ratio.
func resizeFit(img image.Image, size int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w >= h {
		return imaging.Resize(img, size, 0, imaging.Lanczos)
	}
	return imaging.Resize(img, 0, size, imaging.Lanczos)
}
