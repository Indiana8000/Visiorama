package ai

// AnalyzeRequest is sent to visiorama-ai for a single media item.
type AnalyzeRequest struct {
	MediaID  int64  `json:"mediaId"`
	FilePath string `json:"filePath"`
	// MediaType is "image" or "video" (poster frame extracted for video).
	MediaType string `json:"mediaType"`
}

// AnalyzeResponse is returned by visiorama-ai for a single media item.
type AnalyzeResponse struct {
	MediaID int64   `json:"mediaId"`
	Labels  []Label `json:"labels"`
	Faces   []Face  `json:"faces"`
}

// Label is a detected object/animal/scene class.
type Label struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	// Source is "yolo" or "classifier".
	Source string  `json:"source"`
	BBox   *BBox   `json:"bbox,omitempty"`
}

// Face is a detected face with its embedding.
type Face struct {
	BBox      BBox      `json:"bbox"`
	Embedding []float32 `json:"embedding"`
	// CropPath is the path to the stored face crop JPEG on the sidecar's filesystem.
	CropPath string `json:"cropPath"`
}

// BBox is a bounding box in pixels relative to the original image dimensions.
type BBox struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// StatusResponse is returned by GET /health on visiorama-ai.
type StatusResponse struct {
	Available    bool     `json:"available"`
	Version      string   `json:"version"`
	LoadedModels []string `json:"loadedModels"`
	QueueDepth   int      `json:"queueDepth"`
	Workers      int      `json:"workers"`
}
