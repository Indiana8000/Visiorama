package main

import (
	"context"
	"fmt"

	"github.com/Indiana8000/visiorama/internal/ai"
)

// runYOLO runs YOLOv8 object detection on the given image file.
// Returns detected labels with bounding boxes and confidence scores.
//
// TODO (Epic I-2): replace stub with real onnxruntime-go inference.
func runYOLO(_ context.Context, modelPath, imagePath string) ([]ai.Label, error) {
	// Stub: verify model and image exist, return empty result.
	if !fileExists(modelPath) {
		return nil, fmt.Errorf("model not found: %s", modelPath)
	}
	if !fileExists(imagePath) {
		return nil, fmt.Errorf("image not found: %s", imagePath)
	}
	// Real implementation will:
	//   1. Load image, resize to 640x640, normalize to [0,1]
	//   2. Run ONNX session with YOLOv8n model
	//   3. Post-process: NMS, filter by confidence threshold
	//   4. Map class indices to COCO label names
	return []ai.Label{}, nil
}

// runFacePipeline runs face detection (RetinaFace) followed by embedding (ArcFace).
// Returns detected faces with bounding boxes and 512d embeddings.
//
// TODO (Epic I-3): replace stub with real onnxruntime-go inference.
func runFacePipeline(_ context.Context, detectorPath, embeddingPath, imagePath string) ([]ai.Face, error) {
	if !fileExists(detectorPath) {
		return nil, fmt.Errorf("detector model not found: %s", detectorPath)
	}
	if !fileExists(embeddingPath) {
		return nil, fmt.Errorf("embedding model not found: %s", embeddingPath)
	}
	if !fileExists(imagePath) {
		return nil, fmt.Errorf("image not found: %s", imagePath)
	}
	// Real implementation will:
	//   1. RetinaFace: detect face bounding boxes + landmarks
	//   2. Align each face crop using landmarks (5-point affine transform)
	//   3. ArcFace: embed each 112x112 aligned crop → 512d float32 vector
	//   4. L2-normalize embedding
	//   5. Save face crop JPEG, return Face{BBox, Embedding, CropPath}
	return []ai.Face{}, nil
}
