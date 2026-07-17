package convert

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"os/exec"

	"github.com/disintegration/imaging"
)

// ToJPEG converts srcPath to JPEG bytes, resizing so the longest side <= maxDim.
// Tries ImageMagick first, falls back to FFmpeg, then Go's native decoder.
func ToJPEG(srcPath string, maxDim int) ([]byte, error) {
	img, err := decode(srcPath)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}

	img = fitMaxDim(img, maxDim)

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(88)); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// OpenImage decodes any supported image format (JPEG, PNG, TIFF, HEIC, …)
// using Go's native decoder first, then ImageMagick, then FFmpeg as fallbacks.
// EXIF orientation is applied automatically.
func OpenImage(srcPath string) (image.Image, error) {
	return decode(srcPath)
}

func decode(srcPath string) (image.Image, error) {
	// Try Go native first (fast path for common formats)
	if img, err := imaging.Open(srcPath, imaging.AutoOrientation(true)); err == nil {
		return img, nil
	}

	// ImageMagick
	if isAvailable("magick") {
		if img, err := decodeViaImageMagick(srcPath); err == nil {
			return img, nil
		}
	}

	// FFmpeg
	if isAvailable("ffmpeg") {
		if img, err := decodeViaFFmpeg(srcPath); err == nil {
			return img, nil
		}
	}

	return nil, fmt.Errorf("no decoder available for %s", srcPath)
}

func decodeViaImageMagick(srcPath string) (image.Image, error) {
	tmp, err := os.CreateTemp("", "imgconv-*.jpg")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	cmd := exec.Command("magick", srcPath, "-flatten", tmp.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("magick: %w — %s", err, string(out))
	}
	return imaging.Open(tmp.Name(), imaging.AutoOrientation(true))
}

func decodeViaFFmpeg(srcPath string) (image.Image, error) {
	tmp, err := os.CreateTemp("", "imgconv-*.jpg")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	cmd := exec.Command("ffmpeg", "-y", "-noautorotate", "-i", srcPath, "-vframes", "1", "-q:v", "2", tmp.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w — %s", err, string(out))
	}
	return imaging.Open(tmp.Name(), imaging.AutoOrientation(true))
}

func fitMaxDim(img image.Image, maxDim int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return img
	}
	if w >= h {
		return imaging.Resize(img, maxDim, 0, imaging.Lanczos)
	}
	return imaging.Resize(img, 0, maxDim, imaging.Lanczos)
}

func isAvailable(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
