# ADR-007: AI Recognition Architecture

- Status: Proposed
- Date: 2026-07-17

## Context
Users want to find photos by person, animal, and object. EXIF metadata alone cannot answer
"show all photos with Andreas" or "show photos with dogs".

## Decisions

### Inference: Local Only
No cloud APIs. All inference runs on-device. Feature degrades gracefully if AI binary absent
(same pattern as ffmpeg for transcode).

### Hardware Target: CPU Only
No GPU assumption. Models selected for CPU-viable latency (~500ms–1s per image on x86).
Acceptable because inference runs as background post-scan queue, not real-time.

### Deployment: Optional Separate Binary `visiorama-ai`
- Compiled Go binary using CGO + onnxruntime bindings.
- Distributed separately; main binary detects it via config path or PATH.
- If absent: AI features disabled, no errors.
- Models downloaded automatically on first run (configurable model dir).

### Recognition Pipeline (per image)
1. **Object/Animal Detection** — YOLOv8n ONNX (~80 COCO classes, 50–150ms/image)
2. **Fine-grained Classification** — EfficientNet-B0 ONNX for breeds etc. (200–500ms/image)
3. **Face Detection** — MTCNN or RetinaFace ONNX (bounding boxes)
4. **Face Embedding** — ArcFace/FaceNet ONNX (128-512d vector, 100–300ms/image)

### Face Identity: Cluster-then-Name Enrollment
- Embeddings clustered with DBSCAN after initial analysis.
- User reviews clusters (grid of face thumbnails), names each cluster.
- User can remove false positives before confirming.
- Split clusters: user can merge by naming both the same.
- Corrections possible post-enrollment: via Persons Gallery and Lightbox detail panel.

### Trigger: Automatic Post-Scan
- New/changed media from scan → enqueued for AI analysis automatically.
- No separate manual trigger needed (future: optional manual re-analyze button).

### UI
- **Persons Gallery** — new top-level view, navigation button next to Map button.
  Shows person tiles (representative face crop, name, photo count).
- **Lightbox Detail Panel** — "People" and "Labels" rows in existing metadata section.
  Each person/label is a link to Persons Gallery filtered view.
  Corrections: ✕ button per person removes face assignment.
- **Progress Badge** — Persons button shows "Analysing… 234/1500" during active queue.

## Consequences
- Requires `visiorama-ai` binary + model download (~300–500MB) for feature to activate.
- `visiorama-ai` communicates with main service via HTTP (localhost) or Unix socket.
- New DB tables: `ai_labels`, `ai_faces`, `ai_persons`, `ai_embeddings`.
- Face crops stored as small JPEG files in service-owned cache dir.

## Rejected Alternatives
- Python sidecar: extra runtime dependency, complex deployment.
- Cloud APIs: privacy concern, requires internet, ongoing cost.
- Models embedded in binary: ~500MB binary, impractical.
- GPU-only models: excludes target hardware.
