# Epic I: AI Recognition — Person, Animal & Object Detection

**ADR:** ADR-007  
**Status:** Planned  
**Priority:** Post-V1

---

## Architecture Summary

- **Inference:** Local only, CPU-only, optional `visiorama-ai` binary
- **Models:** Auto-downloaded ONNX on first run
- **Trigger:** Automatic after every scan (new/changed media)
- **Fallback:** Feature silently disabled if `visiorama-ai` absent (like ffmpeg)
- **Communication:** `visiorama-ai` ↔ main service via localhost HTTP or Unix socket

---

## Epic Breakdown

### I-1 — `visiorama-ai` Binary Foundation (P0 for epic)

Build the optional Go binary with ONNX runtime.

- [ ] Go module `visiorama-ai` with CGO + `onnxruntime-go` bindings
- [ ] HTTP server: `/analyze` endpoint accepts media path, returns labels+faces JSON
- [ ] Model manager: auto-download models to configured `ai.modelDir` on first use
- [ ] Health endpoint: reports loaded models, queue depth
- [ ] Config: `ai.binary`, `ai.modelDir`, `ai.socketPath`
- [ ] Main binary detects `visiorama-ai` at startup, sets `aiAvailable = true/false`
- [ ] CI: separate build target in release.yml (linux/amd64, arm64)

Acceptance:
- `visiorama-ai` absent → no errors, AI features hidden in UI
- `visiorama-ai` present → models download on first analysis

---

### I-2 — Object & Animal Detection Pipeline (P0 for epic)

- [ ] Integrate YOLOv8n ONNX (80 COCO classes)
- [ ] Integrate EfficientNet-B0 fine-grained classifier (breeds, vehicle types)
- [ ] Output: `[{label: "dog", confidence: 0.94, bbox: [x,y,w,h]}, ...]`
- [ ] Confidence threshold configurable (`ai.labelMinConfidence`, default 0.6)
- [ ] Store results in `ai_labels` table: `media_id, label, confidence, source`

Acceptance:
- Photo with dog → label "dog" (≥0.6 confidence)
- Photo with Golden Retriever → additional label "Golden Retriever"

---

### I-3 — Face Detection & Embedding Pipeline (P0 for epic)

- [ ] Integrate MTCNN or RetinaFace ONNX for face detection (bounding boxes)
- [ ] Integrate ArcFace ONNX for 512d face embeddings
- [ ] Crop face thumbnails → store as JPEG in `ai.faceCacheDir`
- [ ] Store in `ai_faces` table: `media_id, bbox_json, embedding_blob, crop_path`
- [ ] Min face size threshold (skip tiny/blurry faces, configurable pixels)

Acceptance:
- Group photo → N face entries detected and embedded
- Very small/blurry faces → skipped, not stored

---

### I-4 — Analysis Queue & Scheduler (P0 for epic)

- [ ] New DB table `ai_jobs`: `media_id, status, queued_at, finished_at, error`
- [ ] After scan completes: enqueue all new/changed `media_id` into `ai_jobs`
- [ ] `visiorama-ai` polls queue (or main service pushes via socket)
- [ ] Bounded concurrency (configurable `ai.workers`, default 2)
- [ ] Retry on transient error (max 3 attempts)
- [ ] Progress tracking: `analyzed_count`, `total_queued` exposed via `/api/ai/status`

Acceptance:
- 1000 new images after scan → all enqueued automatically
- Main service exposes queue progress for UI badge

---

### I-5 — Cluster & Enrollment UI (P0 for faces)

Flow: detect → embed → cluster → user names → persist identities

- [ ] DBSCAN clustering on `ai_faces.embedding_blob` (run after queue drains)
- [ ] New DB tables:
  - `ai_persons`: `id, name, cover_face_id, created_at`
  - `ai_face_assignments`: `face_id, person_id, confirmed` (confirmed = user-named vs auto)
- [ ] API: `GET /api/ai/clusters` — returns unreviewed clusters with face crop URLs
- [ ] API: `POST /api/ai/persons` — name a cluster, creates person
- [ ] API: `DELETE /api/ai/clusters/{clusterId}/faces/{faceId}` — remove face from cluster before naming
- [ ] API: `POST /api/ai/persons/{personId}/merge/{otherId}` — merge two persons
- [ ] New view: **Cluster Review** — accessible from Persons Gallery when unreviewed clusters exist
  - Grid of clusters, each showing 6 representative face thumbnails
  - Name input per cluster, remove-false-positive button per face

Acceptance:
- 3 clusters for "Andreas" across lighting/angle → user names all three "Andreas" → merged
- False positive removed before naming → not assigned

---

### I-6 — Persons Gallery View (P0 for UI)

New top-level view, button next to Map in navigation.

- [ ] Route: `/persons`
- [ ] API: `GET /api/ai/persons` — list all named persons with cover crop URL + media count
- [ ] API: `GET /api/ai/persons/{personId}/media` — paged media list for a person
- [ ] View: person tiles (face crop, name, count) — same tile style as albums
- [ ] Click person → media grid of all photos containing that person
- [ ] Progress badge on nav button: "Analysing… 234/1500" during active queue
- [ ] Badge: "N clusters to review" when unreviewed clusters exist

Acceptance:
- Persons Gallery shows all named persons
- Clicking "Andreas" shows all photos with Andreas across all albums

---

### I-7 — Lightbox Detail Integration (P1)

Extend existing metadata panel in LightboxView.

- [ ] API: `GET /api/media/{mediaId}/ai` — returns `{ persons: [...], labels: [...] }`
- [ ] Metadata panel: new "People" row — names as chips, each links to Persons Gallery
- [ ] Metadata panel: new "Labels" row — object/animal labels as chips (e.g. "dog · car · tree")
- [ ] Per-person ✕ button — removes face assignment for this media item
  - API: `DELETE /api/ai/media/{mediaId}/persons/{personId}`
- [ ] Correction from Lightbox: reassign face to different person
  - API: `PUT /api/ai/media/{mediaId}/faces/{faceId}/person/{personId}`

Acceptance:
- Photo with Andreas + dog → "People: Andreas" + "Labels: dog"
- ✕ on Andreas → removed from this photo, not from other photos

---

### I-8 — Re-analysis & Maintenance (P2)

- [ ] Manual "Re-analyze all" button in admin/settings
- [ ] Per-album re-analyze option
- [ ] Config: `ai.reanalyzeOnFullScan` (default false — only new/changed)
- [ ] Cleanup: orphaned face crops removed when media deleted (orphan scan hook)
- [ ] `visiorama-ai` version check — warn if binary is outdated vs main service

---

## Database Schema (New Tables)

```sql
CREATE TABLE ai_jobs (
    media_id     INTEGER NOT NULL REFERENCES media(id),
    status       TEXT NOT NULL CHECK(status IN ('queued','running','success','failed')),
    attempts     INTEGER NOT NULL DEFAULT 0,
    queued_at    TEXT NOT NULL,
    finished_at  TEXT,
    error        TEXT,
    PRIMARY KEY (media_id)
);

CREATE TABLE ai_labels (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id     INTEGER NOT NULL REFERENCES media(id),
    label        TEXT NOT NULL,
    confidence   REAL NOT NULL,
    source       TEXT NOT NULL  -- 'yolo', 'classifier'
);

CREATE TABLE ai_faces (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id     INTEGER NOT NULL REFERENCES media(id),
    bbox_json    TEXT NOT NULL,   -- {"x":…,"y":…,"w":…,"h":…}
    embedding    BLOB NOT NULL,   -- 512 float32 values
    crop_path    TEXT NOT NULL
);

CREATE TABLE ai_persons (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    cover_face_id INTEGER REFERENCES ai_faces(id),
    created_at   TEXT NOT NULL
);

CREATE TABLE ai_face_assignments (
    face_id      INTEGER NOT NULL REFERENCES ai_faces(id),
    person_id    INTEGER NOT NULL REFERENCES ai_persons(id),
    confirmed    INTEGER NOT NULL DEFAULT 0,  -- 1 = user-confirmed
    PRIMARY KEY (face_id)
);

CREATE INDEX idx_ai_labels_media   ON ai_labels(media_id);
CREATE INDEX idx_ai_faces_media    ON ai_faces(media_id);
CREATE INDEX idx_ai_assignments_person ON ai_face_assignments(person_id);
```

---

## Config Extension

```yaml
ai:
  binary: ""           # path to visiorama-ai binary, empty = auto-detect from PATH
  modelDir: ""         # model storage dir, empty = <dataDir>/models
  faceCacheDir: ""     # face crop cache, empty = <dataDir>/faces
  workers: 2           # concurrent inference workers
  labelMinConfidence: 0.6
  faceMinPixels: 40    # minimum face dimension in pixels
  reanalyzeOnFullScan: false
```

---

## API Surface (New Endpoints)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/ai/status` | Queue depth, analyzed count, ai binary available |
| GET | `/api/media/{id}/ai` | Labels + persons for one media item |
| GET | `/api/ai/persons` | List all named persons |
| GET | `/api/ai/persons/{personId}/media` | Paged media for a person |
| POST | `/api/ai/persons` | Create person from cluster |
| PUT | `/api/ai/persons/{personId}` | Rename person |
| DELETE | `/api/ai/persons/{personId}` | Delete person (keeps faces, clears assignments) |
| POST | `/api/ai/persons/{personId}/merge/{otherId}` | Merge two persons |
| GET | `/api/ai/clusters` | Unreviewed face clusters |
| DELETE | `/api/ai/clusters/{clusterId}/faces/{faceId}` | Remove face from cluster |
| DELETE | `/api/ai/media/{mediaId}/persons/{personId}` | Remove person from media |
| PUT | `/api/ai/media/{mediaId}/faces/{faceId}/person/{personId}` | Reassign face |

---

## Implementation Order

1. I-1 (binary + comms foundation)
2. I-2 (labels pipeline)
3. I-4 (queue + scheduler)
4. I-3 (face pipeline)
5. I-5 (cluster + enrollment)
6. I-6 (persons gallery)
7. I-7 (lightbox integration)
8. I-8 (maintenance)

---

## Open Questions

- ONNX runtime version compatibility across linux/amd64 + arm64 + armv7
- DBSCAN epsilon parameter — needs tuning against real photo set
- Face crop storage budget (estimate: ~5KB/face, 10k faces = 50MB)
