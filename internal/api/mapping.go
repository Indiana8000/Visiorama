package api

import (
	"fmt"

	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

func repoMediaToSummary(m *repositories.Media) MediaSummary {
	return MediaSummary{
		ID:           m.ID,
		AlbumID:      m.AlbumID,
		Filename:     m.Filename,
		Type:         m.Type,
		Width:        m.Width,
		Height:       m.Height,
		DurationMs:   m.DurationMs,
		SizeBytes:    m.SizeBytes,
		CaptureDate:  m.CaptureDate,
		ThumbnailURL: fmt.Sprintf("/api/media/%d/thumbnail", m.ID),
	}
}

func repoMediaToMetadata(m *repositories.Media, warningLarge bool) MediaMetadata {
	return MediaMetadata{
		MediaSummary:     repoMediaToSummary(m),
		Extension:        m.Extension,
		MimeType:         m.MimeType,
		CameraModel:      m.CameraModel,
		LensModel:        m.LensModel,
		GpsLat:           m.GpsLat,
		GpsLon:           m.GpsLon,
		Orientation:      m.Orientation,
		WarningLargeMedia: warningLarge,
	}
}
