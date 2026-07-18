# ADR-007: AI Recognition Architecture

- Status: Implemented
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
- Requires `visiorama-ai` binary + model download (~300 MB) for feature to activate.
- `visiorama-ai` communicates with main service via Unix socket.
- DB tables: `ai_jobs`, `ai_labels`, `ai_faces`, `ai_persons`, `ai_face_assignments`.
- Face crops stored as JPEG files in `ai.faceCacheDir`.
- Alpine Linux requires `gcompat` (glibc shim) because the CGO binary links glibc.
- `install.sh` installs `gcompat` + `onnxruntime` automatically and creates the missing
  `libonnxruntime.so` symlink (Alpine ships only the versioned `.so.1`).

## Implementation Deviations from Plan
- Fine-grained EfficientNet-B0 classifier (breeds etc.) not implemented — YOLOv8n 80-class
  output deemed sufficient for initial release.
- Face detector is SCRFD-10G (not MTCNN/RetinaFace as originally considered).
- Embedding model is ArcFace R100 (`glintr100.onnx`, ~260 MB); plan was to switch to
  `w600k_mbf.onnx` (~12 MB) — not yet done (URL unconfirmed).

## Open Items
- **Model checksums:** ✅ SHA256 hashes populated in `cmd/visiorama-ai/models.go` for all
  three models (yolov8n, SCRFD, ArcFace).
- **Model size:** `glintr100.onnx` (~260 MB) should be replaced by `w600k_mbf.onnx` (~12 MB)
  once a stable, versioned download URL is confirmed.
- **CI build:** `visiorama-ai` CGO binary not yet built in GitHub Actions. Required for
  `install.sh` to download it from Releases. See ADR-005.
- **Cover face selection:** cluster cover face is picked from unsorted Go map iteration in
  `SaveClusterAssignments` → non-deterministic. Should use `MIN(face_id)`.
- **Version check:** no mechanism to warn when `visiorama-ai` binary is outdated relative
  to the main service.

## Rejected Alternatives
- Python sidecar: extra runtime dependency, complex deployment.
- Cloud APIs: privacy concern, requires internet, ongoing cost.
- Models embedded in binary: ~500 MB binary, impractical.
- GPU-only models: excludes target hardware.
