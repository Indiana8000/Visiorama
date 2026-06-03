package util

import "mime"

// knownTypes maps extensions missing from the Windows MIME registry.
var knownTypes = map[string]string{
	".heic": "image/heic",
	".heif": "image/heif",
	".avif": "image/avif",
	".webp": "image/webp",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
	".mkv":  "video/x-matroska",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
}

// RegisterMIMETypes adds known types not reliably present on Windows.
func RegisterMIMETypes() {
	for ext, typ := range knownTypes {
		_ = mime.AddExtensionType(ext, typ)
	}
}

// TypeByExtension returns the MIME type for ext (with leading dot).
func TypeByExtension(ext string) string {
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return knownTypes[ext]
}
