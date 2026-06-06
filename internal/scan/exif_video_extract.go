package scan

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
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
	goexifOK := false
	if x, err := exif.Decode(f); err == nil {
		goexifOK = true
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

	// goexif can't parse HEIC/AVIF ISOBMFF containers — use magick identify for all metadata.
	if !goexifOK {
		extractViaImageMagick(path, m)
	}

	// For formats Go can decode but without EXIF (PNG, WEBP, GIF), get dimensions via DecodeConfig.
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

// extractViaImageMagick fills missing metadata fields in m using "magick identify".
// Used for HEIC/AVIF where goexif can't read the ISOBMFF container.
func extractViaImageMagick(path string, m *repositories.Media) {
	bin, err := exec.LookPath("magick")
	if err != nil {
		return
	}

	// Format string extracts: WxH | datetime | GPS lat | GPS lon | make | model | lens | orientation
	// Fields that are absent in the image return empty strings.
	const format = "%wx%h|%[EXIF:DateTimeOriginal]|%[EXIF:GPSLatitude]|%[EXIF:GPSLongitude]|%[EXIF:Make]|%[EXIF:Model]|%[EXIF:LensModel]|%[EXIF:Orientation]"
	out, err := exec.Command(bin, "identify", "-format", format, path).Output()
	if err != nil {
		return
	}

	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) < 8 {
		return
	}

	// Dimensions
	if m.Width == nil || m.Height == nil {
		dims := strings.SplitN(parts[0], "x", 2)
		if len(dims) == 2 {
			if w, err := strconv.Atoi(dims[0]); err == nil {
				if h, err := strconv.Atoi(dims[1]); err == nil {
					m.Width = &w
					m.Height = &h
				}
			}
		}
	}

	// Capture date — ImageMagick format: "2023:07:15 14:32:01"
	if m.CaptureDate == nil || parts[1] != "" {
		if parts[1] != "" {
			if dt, err := time.ParseInLocation("2006:01:02 15:04:05", parts[1], time.UTC); err == nil {
				s := dt.UTC().Format(time.RFC3339)
				m.CaptureDate = &s
			}
		}
	}

	// GPS — ImageMagick returns degrees/minutes/seconds as "51/1, 30/1, 0/1"
	if m.GpsLat == nil && parts[2] != "" && parts[3] != "" {
		if lat, err := parseDMS(parts[2]); err == nil {
			if lon, err := parseDMS(parts[3]); err == nil {
				m.GpsLat = &lat
				m.GpsLon = &lon
			}
		}
	}

	// Camera make+model
	if m.CameraModel == nil {
		make_ := strings.TrimSpace(parts[4])
		model := strings.TrimSpace(parts[5])
		if model != "" {
			m.CameraModel = &model
		} else if make_ != "" {
			m.CameraModel = &make_
		}
	}

	// Lens
	if m.LensModel == nil && strings.TrimSpace(parts[6]) != "" {
		s := strings.TrimSpace(parts[6])
		m.LensModel = &s
	}

	// Orientation
	if m.Orientation == nil && strings.TrimSpace(parts[7]) != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(parts[7])); err == nil {
			m.Orientation = &v
		}
	}
}

// parseDMS converts an EXIF DMS rational string like "51/1, 30/1, 2748/100" to decimal degrees.
func parseDMS(s string) (float64, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return 0, strconv.ErrSyntax
	}
	deg, err := parseRational(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, err
	}
	min, err := parseRational(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, err
	}
	sec, err := parseRational(strings.TrimSpace(parts[2]))
	if err != nil {
		return 0, err
	}
	return deg + min/60 + sec/3600, nil
}

// parseRational parses "numerator/denominator" into float64.
func parseRational(s string) (float64, error) {
	p := strings.SplitN(s, "/", 2)
	if len(p) != 2 {
		return 0, strconv.ErrSyntax
	}
	num, err := strconv.ParseFloat(strings.TrimSpace(p[0]), 64)
	if err != nil {
		return 0, err
	}
	den, err := strconv.ParseFloat(strings.TrimSpace(p[1]), 64)
	if err != nil || den == 0 {
		return 0, strconv.ErrRange
	}
	return num / den, nil
}

func ExtractVideoMeta(path, relPath, filename, ext, mimeType string, albumID int64, largeWarningBytes int64) (*repositories.Media, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	t := info.ModTime().UTC().Format(time.RFC3339)
	m := &repositories.Media{
		AlbumID:      albumID,
		Filename:     filename,
		RelativePath: relPath,
		Type:         "video",
		Extension:    ext,
		MimeType:     mimeType,
		SizeBytes:    info.Size(),
		CaptureDate:  &t,
		MtimeUnix:    info.ModTime().Unix(),
	}

	if bin, err := exec.LookPath("ffprobe"); err == nil {
		// Separate calls avoid mixed csv format issues (stream vs format sections produce trailing commas).

		// Width + height + rotation (side_data Display Matrix)
		if out, err := exec.Command(bin,
			"-v", "error",
			"-select_streams", "v:0",
			"-show_entries", "stream=width,height:stream_side_data=rotation",
			"-of", "default=noprint_wrappers=1:nokey=0",
			path,
		).Output(); err == nil {
			w, h, rot := 0, 0, 0
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if v, ok := strings.CutPrefix(line, "width="); ok {
					w, _ = strconv.Atoi(v)
				} else if v, ok := strings.CutPrefix(line, "height="); ok {
					h, _ = strconv.Atoi(v)
				} else if v, ok := strings.CutPrefix(line, "rotation="); ok {
					rot, _ = strconv.Atoi(v)
				}
			}
			if w > 0 && h > 0 {
				// rotation ±90/±270 means the frame is sideways — swap to logical dimensions
				absRot := rot
				if absRot < 0 {
					absRot = -absRot
				}
				if absRot == 90 || absRot == 270 {
					w, h = h, w
				}
				m.Width = &w
				m.Height = &h
			}
		}

		// Duration
		if out, err := exec.Command(bin,
			"-v", "error",
			"-show_entries", "format=duration",
			"-of", "csv=p=0",
			path,
		).Output(); err == nil {
			if sec, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); err == nil && sec > 0 {
				ms := int64(sec * 1000)
				m.DurationMs = &ms
			}
		}

		// Camera model — tries standard EXIF tags, then QuickTime-specific tags (iPhone MOV)
		if out, err := exec.Command(bin,
			"-v", "error",
			"-show_entries", "format_tags=com.apple.quicktime.model,com.apple.quicktime.make,model,make",
			"-of", "default=noprint_wrappers=1:nokey=0",
			path,
		).Output(); err == nil {
			model, make_ := "", ""
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if v, ok := strings.CutPrefix(line, "TAG:com.apple.quicktime.model="); ok && v != "" {
					model = v
				} else if v, ok := strings.CutPrefix(line, "TAG:model="); ok && v != "" && model == "" {
					model = v
				} else if v, ok := strings.CutPrefix(line, "TAG:com.apple.quicktime.make="); ok && v != "" {
					make_ = v
				} else if v, ok := strings.CutPrefix(line, "TAG:make="); ok && v != "" && make_ == "" {
					make_ = v
				}
			}
			if model != "" {
				m.CameraModel = &model
			} else if make_ != "" {
				m.CameraModel = &make_
			}
		}
	}

	return m, info.Size() >= largeWarningBytes
}
