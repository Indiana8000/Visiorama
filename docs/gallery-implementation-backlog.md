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

### A-3 Natural Sorting ✅
- Natural-name ordering for albums and media.

### A-4 Album Tile View ✅
- Tile: album name, random cover, recursive media count.
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

### C-2 EXIF Orientation Handling ✅
- Orientation applied for correct display.

---

## Epic D: Refresh and Operations

### D-1 Manual Re-Scan from UI ✅
- Scan trigger button with mode selection (full/quick/orphan).
- Live scan status display.

### D-2 Manual Re-Scan from CLI ❌ (P2)
- `visiorama scan --mode full|quick`

### D-3 Health and Logging ✅
- `/api/health` with source root and DB availability.
- Structured scan logs and extraction errors.

### D-4 Exclusion Rules ✅
- Configurable exclude patterns and extension allowlist.
- MIME/content sniff validation.

---

## Epic E: Scale Hardening (100k)

### E-1 Scan Throughput Tuning ✅
- Bounded concurrency (`scan.maxWorkers`), warmer suspended during scan.

### E-2 Query and Listing Optimization ✅
- Indexed SQLite queries, pagination on all list endpoints.

### E-3 Cache Budget Policy 🔄
- Thumbnail cache: manual reset via `/api/reset_thumbs`.
- Transcode cache: TTL-based expiry (hourly cleanup).
- No hard disk budget enforcement yet.

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
- GitHub Actions release build: linux/amd64, arm64, armv7.
- See ADR-005.

---

## Epic H: Technology Baseline

### H-1 Go Service Baseline ✅
### H-2 Video Transcode Pipeline ✅
- On-demand H.264/AAC MP4 re-encoding via ffmpeg.
- User-triggered per item; job queue with TTL cleanup.
- See ADR-006.

### H-3 Image Format Conversion ✅
- On-demand HEIC/TIFF → JPEG conversion with in-memory cache.

---

## ADR Traceability
- ADR-001 → Epic H-1
- ADR-002 → Epic E-5
- ADR-003 → Epic D-1
- ADR-004 → Epic B-1 (superseded by ADR-006)
- ADR-005 → Epic G-1
- ADR-006 → Epic H-2
