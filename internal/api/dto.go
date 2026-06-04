package api

// ErrorResponse is the uniform error envelope for all 4xx/5xx responses.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PageInfo struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"pageSize"`
	TotalItems int  `json:"totalItems"`
	TotalPages int  `json:"totalPages"`
	HasNext    bool `json:"hasNext"`
	HasPrev    bool `json:"hasPrev"`
}

type Album struct {
	ID                 int64  `json:"id"`
	RelativePath       string `json:"relativePath"`
	Name               string `json:"name"`
	ParentAlbumID      *int64 `json:"parentAlbumId"`
	MediaCountDirect   int    `json:"mediaCountDirect"`
	MediaCountRecursive int   `json:"mediaCountRecursive"`
	ChildAlbumCount    int    `json:"childAlbumCount"`
}

type AlbumTile struct {
	Album
	CoverMediaID      *int64  `json:"coverMediaId"`
	CoverThumbnailURL *string `json:"coverThumbnailUrl"`
}

type Breadcrumb struct {
	AlbumID      *int64 `json:"albumId,omitempty"`
	Name         string `json:"name"`
	RelativePath string `json:"relativePath"`
}

type MediaSummary struct {
	ID           int64   `json:"id"`
	AlbumID      int64   `json:"albumId"`
	Filename     string  `json:"filename"`
	Type         string  `json:"type"`
	Width        *int    `json:"width"`
	Height       *int    `json:"height"`
	DurationMs   *int64  `json:"durationMs"`
	SizeBytes    int64   `json:"sizeBytes"`
	CaptureDate  *string `json:"captureDate"`
	ThumbnailURL string  `json:"thumbnailUrl"`
}

type MediaMetadata struct {
	MediaSummary
	Extension        string   `json:"extension"`
	MimeType         string   `json:"mimeType"`
	CameraModel      *string  `json:"cameraModel"`
	LensModel        *string  `json:"lensModel"`
	GpsLat           *float64 `json:"gpsLat"`
	GpsLon           *float64 `json:"gpsLon"`
	Orientation      *int     `json:"orientation"`
	WarningLargeMedia bool     `json:"warningLargeMedia"`
}

type AlbumResponse struct {
	Album       Album        `json:"album"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs"`
	ChildAlbums []AlbumTile  `json:"childAlbums"`
	Media       []MediaSummary `json:"media"`
	Page        PageInfo     `json:"page"`
}

type ScanJob struct {
	ID            string  `json:"id"`
	Mode          string  `json:"mode"`
	Status        string  `json:"status"`
	StartedAt     *string `json:"startedAt"`
	FinishedAt    *string `json:"finishedAt"`
	ScannedFiles  int     `json:"scannedFiles"`
	IndexedFiles  int     `json:"indexedFiles"`
	SkippedFiles  int     `json:"skippedFiles"`
	ErrorCount    int     `json:"errorCount"`
	FallbackToFull bool   `json:"fallbackToFull"`
}

type HealthResponse struct {
	Status             string `json:"status"`
	MediaRootAvailable bool   `json:"mediaRootAvailable"`
	DatabaseAvailable  bool   `json:"databaseAvailable"`
	UptimeSeconds      int64  `json:"uptimeSeconds"`
	ThumbWarmer        ThumbWarmerStatus `json:"thumbWarmer"`
}

type ThumbWarmerStatus struct {
	Running      bool  `json:"running"`
	PendingItems int64 `json:"pendingItems"`
}
