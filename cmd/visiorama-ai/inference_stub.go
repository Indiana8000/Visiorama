//go:build !cgo

package main

import (
	"context"
	"fmt"

	"github.com/Indiana8000/visiorama/internal/ai"
)

// CGO is not available — ONNX inference disabled at compile time.
// The sidecar will start and serve the health endpoint, but all
// inference calls return an explanatory error.

func runYOLO(_ context.Context, modelPath, imagePath string) ([]ai.Label, error) {
	if !fileExists(modelPath) {
		return nil, fmt.Errorf("model not found: %s", modelPath)
	}
	if !fileExists(imagePath) {
		return nil, fmt.Errorf("image not found: %s", imagePath)
	}
	return nil, fmt.Errorf("visiorama-ai built without CGO: ONNX inference unavailable")
}

func runFacePipeline(_ context.Context, detectorPath, embeddingPath, imagePath, _ string) ([]ai.Face, error) {
	if !fileExists(detectorPath) {
		return nil, fmt.Errorf("detector model not found: %s", detectorPath)
	}
	if !fileExists(embeddingPath) {
		return nil, fmt.Errorf("embedding model not found: %s", embeddingPath)
	}
	if !fileExists(imagePath) {
		return nil, fmt.Errorf("image not found: %s", imagePath)
	}
	return nil, fmt.Errorf("visiorama-ai built without CGO: ONNX inference unavailable")
}
