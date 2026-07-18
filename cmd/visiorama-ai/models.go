package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Indiana8000/visiorama/internal/ai"
)

// modelSpec describes a downloadable ONNX model.
type modelSpec struct {
	Name   string
	File   string
	URL    string
	SHA256 string // hex-encoded SHA-256 of the file; empty = skip verification
}

// models lists all models used by visiorama-ai.
// URLs point to stable releases; SHA256 checksums prevent tampered downloads.
// NOTE: When real model URLs are confirmed, replace these placeholders.
var models = []modelSpec{
	{
		Name:   "yolov8n",
		File:   "yolov8n.onnx",
		// YOLOv8 nano — 80 COCO classes, ~6 MB, CPU-viable
		URL:    "https://github.com/ultralytics/assets/releases/download/v8.4.0/yolov8n.onnx",
		SHA256: "b2bc52f40e8e1c532427d5bde3575a5d5b571b739fab2c6df443733ed1589cbd",
	},
	{
		Name:   "scrfd",
		File:   "scrfd_10g_bnkps.onnx",
		// SCRFD-10G face detector — ~16 MB, ~5 ms/image on CPU
		URL:    "https://huggingface.co/DIAMONIK7777/antelopev2/resolve/main/scrfd_10g_bnkps.onnx",
		SHA256: "5838f7fe053675b1c7a08b633df49e7af5495cee0493c7dcf6697200b85b5b91",
	},
	{
		Name:   "arcface",
		File:   "glintr100.onnx",
		// ArcFace R100 on Glint360K — 512d embeddings, ~260 MB, best open-source accuracy
		URL:    "https://huggingface.co/DIAMONIK7777/antelopev2/resolve/main/glintr100.onnx",
		SHA256: "4ab1d6435d639628a6f3e5008dd4f929edf4c4124b1a7169e1048f9fef534cdf",
	},
}

type modelManager struct {
	dir      string
	cropsDir string
	mu       sync.RWMutex
	loaded   []string
}

func newModelManager(dir, cropsDir string) *modelManager {
	return &modelManager{dir: dir, cropsDir: cropsDir}
}

// EnsureModels downloads any missing models and verifies checksums.
func (m *modelManager) EnsureModels(ctx context.Context) error {
	for _, spec := range models {
		path := filepath.Join(m.dir, spec.File)
		if fileExists(path) {
			if err := verifyChecksum(path, spec.SHA256); err != nil {
				slog.Warn("model checksum mismatch, re-downloading", "model", spec.Name, "err", err)
				_ = os.Remove(path)
			} else {
				slog.Info("model ready", "model", spec.Name, "path", path)
				m.markLoaded(spec.Name)
				continue
			}
		}
		slog.Info("downloading model", "model", spec.Name, "url", spec.URL)
		if err := downloadFile(ctx, spec.URL, path); err != nil {
			// Non-fatal: log and continue. Feature degrades gracefully without models.
			slog.Warn("model download failed", "model", spec.Name, "err", err)
			continue
		}
		if err := verifyChecksum(path, spec.SHA256); err != nil {
			slog.Warn("model checksum failed after download", "model", spec.Name, "err", err)
			_ = os.Remove(path)
			continue
		}
		slog.Info("model downloaded", "model", spec.Name)
		m.markLoaded(spec.Name)
	}
	return nil
}

func (m *modelManager) markLoaded(name string) {
	m.mu.Lock()
	m.loaded = append(m.loaded, name)
	m.mu.Unlock()
}

// LoadedModels returns names of models that are ready for inference.
func (m *modelManager) LoadedModels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.loaded))
	copy(out, m.loaded)
	return out
}

// Analyze runs all available inference pipelines on a media item.
// Pipelines that have no loaded model are skipped gracefully.
func (m *modelManager) Analyze(ctx context.Context, req ai.AnalyzeRequest) (*ai.AnalyzeResponse, error) {
	resp := &ai.AnalyzeResponse{MediaID: req.MediaID}

	// --- Object / Animal Detection ---
	if m.modelAvailable("yolov8n") {
		labels, err := runYOLO(ctx, filepath.Join(m.dir, "yolov8n.onnx"), req.FilePath)
		if err != nil {
			slog.Warn("yolo inference failed", "mediaId", req.MediaID, "err", err)
		} else {
			resp.Labels = append(resp.Labels, labels...)
		}
	}

	// --- Face Detection + Embedding ---
	scrfdReady := m.modelAvailable("scrfd")
	arcfaceReady := m.modelAvailable("arcface")
	if scrfdReady && arcfaceReady {
		faces, err := runFacePipeline(ctx,
			filepath.Join(m.dir, "scrfd_10g_bnkps.onnx"),
			filepath.Join(m.dir, "glintr100.onnx"),
			req.FilePath,
			m.cropsDir,
		)
		if err != nil {
			slog.Warn("face pipeline failed", "mediaId", req.MediaID, "err", err)
		} else {
			resp.Faces = append(resp.Faces, faces...)
		}
	}

	return resp, nil
}

func (m *modelManager) modelAvailable(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, n := range m.loaded {
		if n == name {
			return true
		}
	}
	return false
}

// downloadFile downloads url to dest with a 10-minute timeout.
func downloadFile(ctx context.Context, url, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, dest)
}

func verifyChecksum(path, expected string) error {
	if expected == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("checksum mismatch: got %s want %s", got, expected)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
