package thumbs

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/disintegration/imaging"
)

// generateViaFFmpegConvert decodes an unsupported format (HEIC, AVIF, …) to a
// temporary full-resolution JPEG via ffmpeg, then hands off to the Go imaging
// path for scaling and EXIF-orientation correction.
// Two-step avoids all ffmpeg filter-graph / autorotate / stream conflicts.
func generateViaFFmpegConvert(srcPath, cachePath string, size int) (string, error) {
	tmp := cachePath + ".tmp.jpg"
	defer os.Remove(tmp)

	// -noautorotate: prevents ffmpeg from creating an internal rotation filtergraph
	//   from the Display Matrix side data, which would conflict with any -vf filter.
	// No -map: HEIC is a multi-stream container (grid tiles + thumbnail); ffmpeg must
	//   compose them itself to produce the full-resolution image. Forcing -map 0:v:0
	//   picks only the first tile/thumbnail stream and produces a partial image.
	// EXIF orientation tag is preserved in the output JPEG for Go's AutoOrientation.
	cmd := exec.Command("ffmpeg",
		"-y",
		"-noautorotate",
		"-i", srcPath,
		"-vframes", "1",
		"-q:v", "2",
		tmp,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg decode: %w — %s", err, string(out))
	}

	src, err := imaging.Open(tmp, imaging.AutoOrientation(true))
	if err != nil {
		return "", fmt.Errorf("open ffmpeg intermediate: %w", err)
	}

	thumb := resizeFit(src, size)
	if err := imaging.Save(thumb, cachePath, imaging.JPEGQuality(82)); err != nil {
		return "", fmt.Errorf("save thumb: %w", err)
	}
	return cachePath, nil
}

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
		// Go decoder failed — try ffmpeg (handles HEIC, AVIF, etc.)
		if FFmpegAvailable() {
			return generateViaFFmpegConvert(srcPath, cachePath, size)
		}
		return "", fmt.Errorf("open image: %w (ffmpeg not available as fallback)", err)
	}

	thumb := resizeFit(src, size)

	if err := imaging.Save(thumb, cachePath, imaging.JPEGQuality(82)); err != nil {
		return "", fmt.Errorf("save thumb: %w", err)
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
