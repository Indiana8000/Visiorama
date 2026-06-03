package scan

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type MediaType int

const (
	MediaTypeUnknown MediaType = iota
	MediaTypeImage
	MediaTypeVideo
)

var imageExts = map[string]bool{}
var videoExts = map[string]bool{}

func InitExtensions(imageExtensions, videoExtensions []string) {
	for _, e := range imageExtensions {
		imageExts[strings.ToLower(e)] = true
	}
	for _, e := range videoExtensions {
		videoExts[strings.ToLower(e)] = true
	}
}

func Classify(path string, enableMimeSniff bool) (MediaType, string, string) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	mimeType := mime.TypeByExtension("." + ext)

	if imageExts[ext] {
		if mimeType == "" {
			mimeType = "image/" + ext
		}
		return MediaTypeImage, ext, mimeType
	}
	if videoExts[ext] {
		if mimeType == "" {
			mimeType = "video/" + ext
		}
		return MediaTypeVideo, ext, mimeType
	}

	if !enableMimeSniff {
		return MediaTypeUnknown, ext, mimeType
	}

	f, err := os.Open(path)
	if err != nil {
		return MediaTypeUnknown, ext, mimeType
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return MediaTypeUnknown, ext, mimeType
	}
	sniffed := http.DetectContentType(buf[:n])
	if strings.HasPrefix(sniffed, "image/") {
		return MediaTypeImage, ext, sniffed
	}
	if strings.HasPrefix(sniffed, "video/") {
		return MediaTypeVideo, ext, sniffed
	}
	return MediaTypeUnknown, ext, sniffed
}
