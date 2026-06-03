package scan

import (
	"os"
	"time"

	"github.com/rwcarlsen/goexif/exif"

	"github.com/USERNAME/visiorama/internal/index/repositories"
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

	// fallback capture date
	t := info.ModTime().UTC().Format(time.RFC3339)
	m.CaptureDate = &t

	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
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
					m.CameraModel = &s // prefer Model over Make
				}
			}
			if tag, err := x.Get(exif.LensModel); err == nil {
				if s, err := tag.StringVal(); err == nil && s != "" {
					m.LensModel = &s
				}
			}
			if tag, err := x.Get(exif.Orientation); err == nil {
				if vals, err := tag.Int(0); err == nil {
					o := vals
					m.Orientation = &o
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
