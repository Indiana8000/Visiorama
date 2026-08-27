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

### A-6 Reverse Album Lookup ✅
- `POST /api/albums/by-media-ids` — resolve containing albums for a set of media IDs.

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
- `GET /api/map/clusters`, `GET /api/map/style`, `GET /api/map/proxy/{path...}` —
  tile/style proxy so the browser never calls openfreemap.org directly.
- `GET /api/gps-count` — global GPS count independent of album filter.

### C-2 EXIF Orientation Handling ✅
- Orientation applied for correct display.

---

## Epic D: Refresh and Operations

### D-1 Manual Re-Scan from UI ✅
- Three scan modes: Quick, Full, Orphan.
- Buttons disable during active action (any of the three); static text, no label change.
- Progress shown left of buttons; success auto-clears after 5 s, errors persist.
- Stale/hung jobs marked failed at startup so buttons are immediately clickable.

### D-2 Manual Re-Scan from CLI ✅
- `visiorama scan --mode full|quick|orphan` runs synchronously, no HTTP server needed.

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
- Orphaned `ai_labels`/`ai_faces`/`ai_jobs` rows and face-crop JPEGs cleaned up
  automatically after every scan run via the HTTP/UI path (`scan.Runner`).
- Orphaned thumbnail/poster cache files deleted alongside their `media` row
  on all three scan modes (quick/full/orphan).
- **Known gap:** `visiorama scan --mode ...` (CLI) calls the scanners directly,
  bypassing `scan.Runner` — so a cron job using the CLI instead of the UI/API
  still leaves AI rows and crop files behind. Only the CLI-vs-Runner AI-cleanup
  gap remains; thumbnail-cache cleanup happens inside the scanners themselves
  and is unaffected.

### D-6 Rename/Move Detection ❌ (P2)
- **Open:** a renamed or moved file is handled as delete-old + insert-new on
  the next scan, which loses the old `media_id`'s AI face/label associations
  (`ai_faces`/`ai_labels` are keyed by `media_id`, not by file content).
  Noted as a known gap in ADR-003 "Consequences" but not previously tracked
  here. A real fix would match orphaned + newly-discovered files by size/hash
  during scan and preserve `media_id` across the rename.

### D-7 `/api/reset_thumbs` uses GET ✅
- Endpoint is now `DELETE /api/reset_thumbs` (`router.go`, `openapi.v1.yaml`),
  no longer triggerable by a plain GET from crawlers/prefetch/history.

### D-8 Transcode cache not cleaned on media delete ✅
- `MediaRepo.DeleteByPath` now returns the deleted row's ID, and all three
  scan delete sites (`scanner_orphan.go`, `scanner_quick.go`,
  `scanner_full.go`) call `transcode.CleanupMedia(db, mediaID)` right after
  `thumbs.DeleteCached`, removing the `transcode_jobs` row and its cache file
  immediately instead of waiting on the TTL `cleanupLoop`.

---

## Epic E: Scale Hardening (100k)

### E-1 Scan Throughput Tuning ✅
- Bounded concurrency (`scan.maxWorkers`), warmer suspended during scan.

### E-2 Query and Listing Optimization ✅
- Indexed SQLite queries, pagination on all list endpoints.

### E-3 Cache Budget Policy ✅
- Thumbnail cache: manual reset via `/api/reset_thumbs`; mtime-LRU disk budget (`thumbnails.maxCacheMiB`) enforced after each warmer pass.
- Transcode cache: TTL-based expiry (hourly cleanup) plus mtime-LRU disk budget (`transcode.maxCacheMiB`).
- Shared eviction logic in `internal/cache`. `0` = unlimited (default, backwards compatible).

### E-4 Large Media Warning Threshold ✅
- Configurable threshold (default 100 MB), exposed in media metadata.

### E-5 Embedded Index Storage ✅
- SQLite with migrations.

### E-6 Automated Test Coverage ❌ (P1)
- **Open:** only `internal/cache` and `internal/mapview` have `_test.go` files.
  `internal/index`, `internal/index/repositories`, `internal/scan`,
  `internal/transcode`, `internal/ai`, `internal/convert` — all zero coverage —
  are exactly the packages doing irreversible deletes (orphan scan), DB writes
  (AI job persistence), and untrusted-file decoding. A regression in any of
  them currently has no automated signal before it reaches production.

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

### G-2 CI Build for `visiorama-ai` ✅
- Built in `release.yml` for linux/amd64 + linux/arm64 with onnxruntime cross-linking.
- armv7 skipped (no upstream ORT armv7 release).
- `libonnxruntime.so.*` uploaded to GitHub Release alongside the binary.

---

## Epic H: Technology Baseline

### H-1 Go Service Baseline ✅
### H-2 Video Transcode Pipeline ✅
- On-demand H.264/AAC MP4 re-encoding via ffmpeg.
- User-triggered per item; job queue with TTL cleanup.
- See ADR-006.

### H-3 Image Format Conversion ✅
- On-demand HEIC/TIFF → JPEG conversion with in-memory cache.

### H-4 Dead-Code Cleanup ✅
- `internal/health/` and `internal/stream/` deleted (empty stubs, superseded).

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
- SHA256 checksums populated for all three models.
- Decided: keep `glintr100.onnx`, no swap to `w600k_mbf.onnx` (ADR-007 superseded).

### I-4 Analysis queue & scheduler ✅
- `ai_jobs` table; new/changed media enqueued after scan.
- Bounded concurrency (`ai.workers`).
- Failed jobs are not automatically retried; they re-enqueue only when the same
  media is scanned or reanalyzed again (`ON CONFLICT ... WHERE status = 'failed'`).
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
- `GET /api/ai/counts` — job status counts for progress badge.
- `GET /api/ai/crops/{filename}` — serves face crop JPEGs.
- `DELETE /api/ai/faces/{faceId}/person` — unassign a face from its person.

### I-7 Lightbox detail integration ✅
- Persons chips (linked to Persons Gallery) + Labels chips in metadata panel.
- Per-person ✕ button removes face assignment for this photo.
- `GET /api/media/{mediaId}/ai` — labels + faces (with person assignment) for one item.

### I-8 Re-analysis & maintenance ✅
- Re-analyze button: queues all media for AI re-analysis.
- `POST /api/ai/reanalyze?albumPath=...` — HTTP trigger (config flag below is scan-time only).
- `ai.reanalyzeOnFullScan` config flag (default false).
- `POST /api/ai/cleanup` — removes orphaned `ai_labels`/`ai_faces`/`ai_jobs` DB rows and
  their face-crop JPEG files on disk.
- Decided: no version-check/outdated-binary warning needed — `visiorama-ai` always deployed together with `visiorama` (see G-1/G-2).
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
