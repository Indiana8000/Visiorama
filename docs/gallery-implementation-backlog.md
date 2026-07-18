# Gallery Service Backlog

## Status Legend
- ✅ Implemented
- 🔄 Partial
- ❌ Not started
- P0/P1/P2: Priority if not yet implemented

---

## Epic A: Catalog and Album Navigation

### A-1 Root Path Configuration ✅
- Configurable media root path with startup validation.

### A-2 Recursive Album Discovery ✅
- Every folder is an album, nested hierarchy preserved.
- Empty albums visible.
- `parent_album_id` correctly set for all nesting levels (recursive upsert).

### A-3 Natural Sorting ✅
- Natural-name ordering for albums and media.

### A-4 Album Tile View ✅
- Tile: album name, cover from first media (recursive), recursive media count.
- Missing-cover fallback.

### A-5 Breadcrumb Navigation ✅
- Jump to any ancestor level.

---

## Epic B: Media Presentation

### B-1 Thumbnail Generation and Caching ✅
- On-demand thumbnails in service-owned cache.
- Video poster-frame extraction via ffmpeg.
- Background warmer pre-generates thumbnails.
- Multiple size variants (configurable).

### B-2 Media Grid with Paging ✅
- Paged media grid, large albums remain responsive.

### B-3 Lightbox ✅
- Full-view image/video, prev/next navigation.
- Keyboard and touch navigation.
- Persons chips + Labels chips in metadata panel.
- Face-click in Cluster Review opens photo in Lightbox.

### B-4 Slideshow ✅
- Auto-advance slideshow with pause/resume.
- Fullscreen mode.

---

## Epic C: Metadata and Correct Rendering

### C-1 Metadata Extraction ✅
- Filename, capture date, size, resolution, duration, camera/lens, GPS extracted during scan.

### C-1a GPS Map View ✅
- GPS coordinates clustered and displayed on MapLibre GL map.
- Filter by album.
- Map button shown only when GPS data exists; badge with count.
- Consistent GPS count between AlbumView and PersonsView.

### C-2 EXIF Orientation Handling ✅
- Orientation applied for correct display.

---

## Epic D: Refresh and Operations

### D-1 Manual Re-Scan from UI ✅
- Three scan modes: Quick, Full, Orphan.
- Buttons disable during active action (any of the three); static text, no label change.
- Progress shown left of buttons; success auto-clears after 5 s, errors persist.
- Stale/hung jobs marked failed at startup so buttons are immediately clickable.

### D-2 Manual Re-Scan from CLI ❌ (P2)
- `visiorama scan --mode full|quick`

### D-3 Health and Logging ✅
- `/api/health` with source root and DB availability.
- Structured scan logs and extraction errors.

### D-4 Exclusion Rules ✅
- Configurable exclude patterns and extension allowlist.
- MIME/content sniff validation.

### D-5 Orphan Cleanup ✅
- Deleted media files removed from DB on orphan scan.
- Deleted album directories removed from DB on orphan scan.
- Accessing a deleted album via UI triggers orphan scan automatically.
- Accessing a deleted media item via UI triggers orphan scan automatically.

---

## Epic E: Scale Hardening (100k)

### E-1 Scan Throughput Tuning ✅
- Bounded concurrency (`scan.maxWorkers`), warmer suspended during scan.

### E-2 Query and Listing Optimization ✅
- Indexed SQLite queries, pagination on all list endpoints.

### E-3 Cache Budget Policy 🔄
- Thumbnail cache: manual reset via `/api/reset_thumbs`.
- Transcode cache: TTL-based expiry (hourly cleanup).
- **Open:** no hard disk budget enforcement; LRU eviction not implemented.

### E-4 Large Media Warning Threshold ✅
- Configurable threshold (default 100 MB), exposed in media metadata.

### E-5 Embedded Index Storage ✅
- SQLite with migrations.

---

## Epic F: UX and Accessibility

### F-1 Mobile-First Responsiveness ✅
### F-2 English-Only Copy ✅
### F-3 Keyboard Navigation ✅

---

## Epic G: Packaging and Deployment

### G-1 Distribution Install Script ✅
- `install.sh` downloads main + AI binary from GitHub Releases.
- Auto-detects init system (systemd / OpenRC), writes service units for both binaries.
- Alpine: installs `gcompat` + `onnxruntime`, creates `libonnxruntime.so` symlink.
- Config written once with full inline comments; never overwritten on upgrade.
- See ADR-005.

### G-2 CI Build for `visiorama-ai` ❌ (P0 for release)
- CGO binary with ONNX not yet built in CI.
- Needed: separate GitHub Actions job for linux/amd64 + arm64 with onnxruntime linking.
- See ADR-005 open items.

---

## Epic H: Technology Baseline

### H-1 Go Service Baseline ✅
### H-2 Video Transcode Pipeline ✅
- On-demand H.264/AAC MP4 re-encoding via ffmpeg.
- User-triggered per item; job queue with TTL cleanup.
- See ADR-006.

### H-3 Image Format Conversion ✅
- On-demand HEIC/TIFF → JPEG conversion with in-memory cache.

### H-4 Dead-Code Cleanup ❌ (P2)
- `internal/health/` and `internal/stream/` are empty stub packages, never imported.
- Safe to delete (superseded by `handlers_health.go` and `handlers_media.go`).

---

## Epic I: AI Recognition

### I-1 `visiorama-ai` binary foundation ✅
- CGO binary with onnxruntime-go, Unix socket HTTP server.
- Model manager: auto-download on first run, checksum verification (when hashes provided).
- Health endpoint reports loaded models and queue depth.

### I-2 Object & animal detection pipeline ✅
- YOLOv8n ONNX — 80 COCO classes, ~6 MB.
- Results stored in `ai_labels` table.
- Confidence threshold configurable (`ai.labelMinConfidence`, default 0.6).

### I-3 Face detection & embedding pipeline ✅
- SCRFD-10G face detector + ArcFace R100 embedding.
- Face crops stored as JPEG in `ai.faceCacheDir`.
- Results stored in `ai_faces` table.
- Min face size configurable (`ai.faceMinPixels`, default 40 px).
- **Open:** `glintr100.onnx` (~260 MB) should be replaced by `w600k_mbf.onnx` (~12 MB) — see ADR-007.
- SHA256 checksums populated for all three models.

### I-4 Analysis queue & scheduler ✅
- `ai_jobs` table; new/changed media enqueued after scan.
- Bounded concurrency (`ai.workers`), retry on transient errors.
- Progress exposed via `/api/ai/status`.

### I-5 Cluster & enrollment UI ✅
- DBSCAN clustering on embeddings; re-clustered on each `GET /api/ai/clusters` call.
- Cluster order stable: sorted by minimum face_id (face_id is permanent across re-clusterings).
- UI: grid of face crops per cluster, name input, remove-face button.
- Face crop clickable → opens source photo in Lightbox.
- API: create person, remove face from cluster, merge persons.

### I-6 Persons Gallery view ✅
- Route `/persons`, person tiles (face crop, name, count).
- Click person → media grid.
- Map button shown only when GPS data exists (same badge logic as AlbumView).
- Rename + delete person.

### I-7 Lightbox detail integration ✅
- Persons chips (linked to Persons Gallery) + Labels chips in metadata panel.
- Per-person ✕ button removes face assignment for this photo.

### I-8 Re-analysis & maintenance ✅
- Re-analyze button: queues all media for AI re-analysis.
- `ai.reanalyzeOnFullScan` config flag (default false).
- AI cleanup: removes orphaned face crops and DB entries.
- **Open:** `visiorama-ai` version check / outdated binary warning not implemented.
- Cover face = min face_id (sorted before insert; stable across re-clusterings).

---

## ADR Traceability
- ADR-001 → Epic H-1
- ADR-002 → Epic E-5
- ADR-003 → Epic D-1
- ADR-004 → Epic B-1 (superseded by ADR-006)
- ADR-005 → Epic G-1, G-2
- ADR-006 → Epic H-2
- ADR-007 → Epic I
