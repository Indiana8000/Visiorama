package main

import (
	"archive/zip"
	"bytes"
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
// If ZipURL is set the model is downloaded as a ZIP; ZipFiles lists the entries
// to extract (all must land in the same directory as File).
type modelSpec struct {
	Name     string
	File     string
	URL      string
	SHA256   string   // hex SHA-256 of File after extraction; empty = skip
	ZipURL   string   // download a ZIP instead of a bare file
	ZipFiles []string // entries to extract from the ZIP (relative names inside ZIP)
}

// models lists all models used by visiorama-ai.
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
	{
		Name: "species",
		File: "mobilenet_v3_small.onnx",
		// MobileNetV3-Small (Qualcomm AI Hub) — ImageNet-1000, ~10 MB zip, ~2 ms/crop on CPU.
		// Uses ONNX external-data format: mobilenet_v3_small.data holds the weights.
		// Dog breed determined by renormalising softmax over the ~99 dog-class subset.
		ZipURL: "https://qaihub-public-assets.s3.us-west-2.amazonaws.com/qai-hub-models/models/mobilenet_v3_small/releases/v0.58.0/mobilenet_v3_small-onnx-float.zip",
		ZipFiles: []string{
			"mobilenet_v3_small.onnx",
			"mobilenet_v3_small.data",
		},
		SHA256: "cfa0684a4290a63593adb26b8fb036840f1fbb678831e93d50c056916beabdc4",
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

		// Download
		var dlErr error
		if spec.ZipURL != "" {
			slog.Info("downloading model (zip)", "model", spec.Name, "url", spec.ZipURL)
			dlErr = downloadZip(ctx, spec.ZipURL, m.dir, spec.ZipFiles)
		} else if spec.URL != "" {
			slog.Info("downloading model", "model", spec.Name, "url", spec.URL)
			dlErr = downloadFile(ctx, spec.URL, path)
		} else {
			slog.Warn("model not found and no download URL configured", "model", spec.Name, "path", path)
			continue
		}
		if dlErr != nil {
			slog.Warn("model download failed", "model", spec.Name, "err", dlErr)
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

			// --- Species / Breed Classification (dog, cat, bird) ---
			// Only runs per YOLO class when that class was detected — negligible overhead otherwise.
			if m.modelAvailable("species") {
				speciesModel := filepath.Join(m.dir, "mobilenet_v3_small.onnx")
				type speciesTarget struct {
					cls string
					m   map[int]string
				}
				for _, t := range []speciesTarget{
					{"dog", dogBreedMap},
					{"cat", catSpeciesMap},
					{"bird", birdSpeciesMap},
				} {
					sl, sErr := runSpeciesForLabels(ctx, speciesModel, req.FilePath, labels, t.cls, t.m)
					if sErr != nil {
						slog.Warn("species inference failed", "mediaId", req.MediaID, "class", t.cls, "err", sErr)
					} else {
						resp.Labels = append(resp.Labels, sl...)
					}
				}
			}
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

// downloadZip downloads a ZIP from url and extracts only the entries listed in
// files into destDir.  Each entry is written atomically via a .tmp sibling.
func downloadZip(ctx context.Context, url, destDir string, files []string) error {
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

	// Read into memory so zip.NewReader can seek. Models are ≤300 MB — acceptable.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read zip body: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	want := make(map[string]bool, len(files))
	for _, f := range files {
		want[f] = true
	}

	for _, zf := range zr.File {
		name := filepath.Base(zf.Name) // strip any directory prefix inside ZIP
		if !want[name] {
			continue
		}
		dest := filepath.Join(destDir, name)
		if err := extractZipEntry(zf, dest); err != nil {
			return fmt.Errorf("extract %s: %w", name, err)
		}
		slog.Info("extracted", "file", name)
		delete(want, name)
	}

	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for k := range want {
			missing = append(missing, k)
		}
		return fmt.Errorf("zip missing entries: %v", missing)
	}
	return nil
}

func extractZipEntry(zf *zip.File, dest string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, rc); err != nil {
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
