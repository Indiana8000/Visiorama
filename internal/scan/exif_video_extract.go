package scan

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	_ "golang.org/x/image/webp"

	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

func ExtractImageMeta(path, relPath, filename, ext, mimeType string, albumID int64, largeWarningBytes int64) (*repositories.Media, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}

	m := &repositories.Media{
		AlbumID:      albumID,
		Filename:     filename,
		RelativePath: relPath,
		Type:         "image",
		Extension:    ext,
		MimeType:     mimeType,
		SizeBytes:    info.Size(),
		MtimeUnix:    info.ModTime().Unix(),
	}

	t := info.ModTime().UTC().Format(time.RFC3339)
	m.CaptureDate = &t

	f, err := os.Open(path)
	if err != nil {
		return m, info.Size() >= largeWarningBytes
	}
	defer f.Close()

	// Try EXIF first (JPEG, TIFF, some HEIC)
	if x, err := exif.Decode(f); err == nil {
		if dt, err := x.DateTime(); err == nil {
			s := dt.UTC().Format(time.RFC3339)
			m.CaptureDate = &s
		}
		if lat, lon, err := x.LatLong(); err == nil {
			m.GpsLat = &lat
			m.GpsLon = &lon
		}
		if tag, err := x.Get(exif.Make); err == nil {
			if s, err := tag.StringVal(); err == nil && s != "" {
				m.CameraModel = &s
			}
		}
		if tag, err := x.Get(exif.Model); err == nil {
			if s, err := tag.StringVal(); err == nil && s != "" {
				m.CameraModel = &s
			}
		}
		if tag, err := x.Get(exif.LensModel); err == nil {
			if s, err := tag.StringVal(); err == nil && s != "" {
				m.LensModel = &s
			}
		}
		if tag, err := x.Get(exif.Orientation); err == nil {
			if v, err := tag.Int(0); err == nil {
				m.Orientation = &v
			}
		}
		if tag, err := x.Get(exif.PixelXDimension); err == nil {
			if v, err := tag.Int(0); err == nil {
				m.Width = &v
			}
		}
		if tag, err := x.Get(exif.PixelYDimension); err == nil {
			if v, err := tag.Int(0); err == nil {
				m.Height = &v
			}
		}
	}

	// Fall back to image.DecodeConfig for formats without EXIF (PNG, WEBP, GIF)
	if m.Width == nil || m.Height == nil {
		if _, err := f.Seek(0, 0); err == nil {
			if cfg, _, err := image.DecodeConfig(f); err == nil && cfg.Width > 0 {
				w, h := cfg.Width, cfg.Height
				m.Width = &w
				m.Height = &h
			}
		}
	}

	return m, info.Size() >= largeWarningBytes
}

func ExtractVideoMeta(path, relPath, filename, ext, mimeType string, albumID int64, largeWarningBytes int64) (*repositories.Media, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	t := info.ModTime().UTC().Format(time.RFC3339)
	return &repositories.Media{
		AlbumID:      albumID,
		Filename:     filename,
		RelativePath: relPath,
		Type:         "video",
		Extension:    ext,
		MimeType:     mimeType,
		SizeBytes:    info.Size(),
		CaptureDate:  &t,
		MtimeUnix:    info.ModTime().Unix(),
	}, info.Size() >= largeWarningBytes
}
