# Gallery Service Plan (Phase 0: Product and Architecture Specification)

## 1. Goal
Build a private, read-only gallery service for home-network usage that displays images and videos from a configurable filesystem root. Every directory is an album, nested directories are nested albums, and users browse level-by-level via clickable tiles.

This document defines product requirements and implementation boundaries. Initial technology direction is captured in the decision log and will be formalized in ADR documents.

## 2. Scope and Context

### In Scope (V1)
- Home-network hosted web application.
- Audience: owner + family members on LAN.
- Configurable root directory (including mounted SMB paths on Linux hosts).
- Album navigation via tiles, one folder level at a time.
- Album tile content: album name + random cover image + media count.
- Empty albums are visible.
- Natural name sorting by default.
- Media grid with thumbnails.
- Lightbox mode with previous/next navigation across current album.
- Video playback in lightbox.
- Metadata display:
  - Filename
  - Capture date
  - File size
  - Resolution
  - Duration (video)
  - Camera/lens
  - GPS (when present)
- EXIF orientation handling (auto-rotate for correct display).
- Manual re-scan support:
  - UI button
  - Server-side CLI trigger
- Thumbnail cache on server is allowed.
- Hidden/system files are excluded by default.
- Supported media extensions are configurable via allowlist.
- Media validation uses extension allowlist and MIME/content sniff checks for safer classification.
- Large media warning threshold is configurable (default: 100 MB).
- Mobile-friendly responsive UI is required.
- English-only UI text.

### Out of Scope (V1)
- Uploading media.
- In-app file editing, moving, renaming, deleting.
- User authentication/authorization.
- Public internet exposure hardening (can be revisited later).
- Real-time filesystem watching.
- GPS map rendering.

## 3. Operating Model

### Deployment Context
- Primary deployment target: Linux server in home network.
- Media root may be local disk or mounted remote share (for example SMB mount).
- Service is read-only against source media.

### Data Freshness Model
- No automatic watch mode in V1.
- Content refresh occurs only after manual re-scan.

## 4. Functional Requirements

### FR-1 Root Configuration
- System must allow configuration of a single media root path.
- System must reject startup if root is unavailable or unreadable.

Acceptance criteria:
- Given a valid path, service indexes and serves media.
- Given an invalid/unmounted path, service exposes clear health/error status.

### FR-2 Album Model and Navigation
- Every folder under root is an album.
- Nested folders must be represented as nested album levels.
- UI must open albums by clicking tiles (no tree sidebar required).
- Empty albums must still be listed.

Acceptance criteria:
- Opening an album shows child albums first and media entries for current level.
- Breadcrumb navigation is present for nested navigation clarity.

### FR-3 Album Tile Representation
- Tile must contain:
  - Album name
  - Randomly selected cover image
  - Number of media items in that album subtree (recursive count)

Acceptance criteria:
- If no eligible image exists, fallback placeholder is shown.
- Cover selection strategy may be pseudo-random and cached to prioritize performance.

### FR-4 Sorting
- Default sorting for albums and media is natural name sorting.

Acceptance criteria:
- Names sort naturally (for example: item2 before item10).

### FR-5 Media Support
- Service should support common image and video formats.
- Unsupported files are ignored without failing indexing.
- Hidden/system files and folders are excluded by default.
- Extension allowlist is configurable.
- MIME/content sniff validation is applied to reduce mismatches from incorrect file extensions.

Acceptance criteria:
- Known supported files appear in gallery.
- Unsupported files do not break listing or scan.
- Changing allowlist configuration affects the next scan result without code changes.

### FR-5a File and Folder Filtering
- System must support configurable exclusion patterns for files/folders.

Acceptance criteria:
- Default exclusions include common system artifacts (for example: dotfiles, @eaDir, Thumbs.db).
- Operator can add additional excludes without code changes.

### FR-6 Thumbnails and Performance
- Server must generate and cache thumbnails/previews.
- Cache must be separate from source media and fully disposable.

Acceptance criteria:
- First view may generate cache; subsequent views are faster.
- Rebuilding cache does not modify source media.

### FR-7 Lightbox
- Full-view lightbox must support:
  - Previous/next navigation within the current album view set.
  - Image display and video playback.

Acceptance criteria:
- Keyboard and touch navigation supported on desktop/mobile.
- Slideshow/auto-play is optional in V1 and enabled only if supported by the chosen lightbox solution with low complexity impact.

### FR-8 Metadata
- Metadata panel must expose required fields when available.
- Missing metadata should be rendered as unavailable, not as errors.

Acceptance criteria:
- EXIF orientation is respected for display.
- Video duration is shown when extractable.
- GPS is shown as raw coordinates in V1.

### FR-9 Re-Scan Triggers
- System must provide:
  - UI trigger for manual re-scan.
  - CLI command to trigger re-scan from host.
- Re-scan modes must include full scan and optional quick scan.

Acceptance criteria:
- Re-scan completes with observable status (running, success, failure).
- Updated filesystem state appears after scan completion.
- UI scan trigger is available to all LAN users (no auth gate in V1).

### FR-9a Scan Modes
- Full scan mode must always be available.
- Quick-scan mode should be available to prioritize performance when only partial changes are expected.
- Quick-scan delta detection should use file and folder modification timestamps.

Acceptance criteria:
- Operator can choose full or quick mode from UI and CLI.
- Full scan remains fallback when quick scan cannot determine delta safely.
- Quick-scan considers modified files/folders based on mtime change.

### FR-9b Index Persistence
- System must persist catalog/index data across restarts.
- Persistent index should support fast startup and low operational overhead.

Acceptance criteria:
- Service restart does not require mandatory full re-index before browsing.
- Index store survives process restart and host reboot.

### FR-10 Read-Only Safety
- Service must not mutate source directories or media files.

Acceptance criteria:
- No write operations to source root except read access.
- Any persistent writes are restricted to service-owned cache/state paths.

### FR-11 Large Media Warning Threshold
- System must show a warning before opening/streaming very large media.
- Threshold must be configurable; default is 100 MB.

Acceptance criteria:
- Items above threshold display warning before playback/full-load.
- Threshold can be changed via configuration.

## 5. Non-Functional Requirements

### NFR-1 Scale
- Target catalog size: up to 100,000 media items.

### NFR-2 Responsiveness
- Album listing and media grid should remain responsive under large datasets.
- Pagination or incremental loading is required for large albums.

### NFR-3 Resource Control
- Thumbnail generation must be bounded (concurrency and memory limits).
- Long scans should provide progress and avoid UI timeouts.

### NFR-4 Reliability
- Temporary path outages (for mounted shares) must fail gracefully.
- Partial metadata extraction failures must not block indexing.

### NFR-5 Mobile UX
- Responsive layout and touch-friendly interactions are mandatory in V1.

### NFR-6 Language
- All UI text and user-facing messages in English only.

## 6. Conceptual Data Model

### Entities
- Album
  - id
  - relativePath
  - name
  - parentAlbumId (nullable)
  - coverMediaId (derived/cached)
  - mediaCountRecursive
  - mediaCountDirect
  - childAlbumCount
- Media
  - id
  - albumId
  - relativePath
  - filename
  - type (image/video)
  - extension
  - sizeBytes
  - width
  - height
  - durationMs (video)
  - captureDate
  - cameraModel
  - lensModel
  - gpsLat
  - gpsLon
  - orientation
  - checksumOrFingerprint (optional, for dedup/incremental strategies)
- ScanJob
  - id
  - startedAt
  - finishedAt
  - status
  - scannedFiles
  - indexedFiles
  - errors

## 7. API Surface (Technology-Agnostic Draft)

### Read APIs
- GET /api/albums/{albumIdOrPath}
  - returns album metadata, child albums, and paged media list
- GET /api/media/{mediaId}/metadata
  - returns metadata for one item
- GET /api/media/{mediaId}/stream
  - image full-size or video stream endpoint
- GET /api/media/{mediaId}/thumbnail?size=...
  - cached thumbnail/preview

### Scan APIs
- POST /api/scan
  - triggers manual re-scan
- GET /api/scan/{jobId}
  - returns progress and result
- POST /api/scan?mode=full|quick
  - triggers selected scan mode

### CLI Interface
- gallery-service scan
  - triggers manual scan job
- gallery-service scan --wait
  - blocks until done and returns status code
- gallery-service scan --mode full|quick
  - selects scan mode

## 8. Performance and Scalability Strategy (Planned)

### Indexing
- Keep a local index for album/media metadata and navigation speed.
- Use batch writes and bounded worker pools during scans.
- Persist index using an embedded local database engine (technology to be finalized in implementation ADR).

### Listing
- Use pagination/infinite scrolling for large album media sets.
- Avoid loading full metadata payload in grid views.

### Thumbnails
- Pre-generate on-demand and cache; optionally prewarm high-use albums later.
- Support size variants to reduce bandwidth and mobile load cost.

### Video
- Start with static thumbnail extraction or poster frame.
- Keep preview strategy minimal and choose the most performant option first.
- V1 uses poster-frame previews only (no animated previews).

## 9. Security and Privacy Baseline (Home Network)
- No auth in V1 by requirement.
- Service should bind to configurable interface/port.
- Path traversal protection required (never escape configured root).
- Metadata exposure is intentional in this private setup, including GPS when available.
- UI actions are available to LAN users by design (trusted network model).

## 10. Observability and Operations
- Structured logs for scan lifecycle and extraction errors.
- Health endpoint includes source root availability state.
- Scan metrics:
  - total files discovered
  - supported media count
  - skipped/unsupported count
  - extraction error count
  - total scan time

## 11. Risks and Mitigations
- Risk: Very large folders degrade UX.
  - Mitigation: pagination/infinite loading, thumbnail-first rendering.
- Risk: Mounted SMB intermittency.
  - Mitigation: resilient scan errors, health status, manual retry.
- Risk: Metadata extraction cost at 100k scale.
  - Mitigation: staged extraction and cached index persistence.
- Risk: Video processing overhead.
  - Mitigation: lightweight poster strategy first, no heavy previews in V1.

## 12. Milestone Backlog (Implementation-Oriented)

### M1: Core Catalog and Navigation
- Root path config
- Recursive scan to album/media index
- Album tile navigation and breadcrumbs
- Natural sorting

Definition of done:
- Browse nested albums and list media from indexed data.

### M2: Rendering and Lightbox
- Thumbnail API + cache
- Media grid UI
- Lightbox with image/video support and next/prev
- Mobile responsive behavior

Definition of done:
- End-to-end browsing and viewing works on desktop and mobile.

### M3: Metadata and Correctness
- EXIF/video metadata extraction
- Metadata panel in UI
- Orientation correction

Definition of done:
- Required metadata fields display when available.

### M4: Manual Re-Scan and Ops
- UI scan trigger
- CLI scan trigger
- Full and quick scan mode support
- Scan status API and logs
- Health and metrics basics

Definition of done:
- User can refresh library state on demand and monitor scan result.

### M5: Hardening for 100k
- Performance profiling
- Concurrency and memory tuning
- Large album pagination tuning

Definition of done:
- Meets responsive browsing target for 100k catalog scenario.

## 13. Open Items for Next Discussion (Technology Decision Session)
- Final list of explicitly supported extensions per media type.
- Metadata extraction libraries/tooling.
- Video streaming strategy details.
- Cache eviction and disk budget policy.
- Alpine full packaging/integration scope for post-V1.
- Backup/rebuild strategy for index and cache.

## 14. Change Control
- Any requirement updates should be appended as dated decisions.
- Keep this document as source of truth for implementation scope before coding starts.

## 15. Decision Log

### 2026-06-03 (Requirements Clarification)
- Empty albums are visible.
- Album tile media count is recursive across subtree.
- Random cover policy is performance-first (implementation-defined randomization with caching/determinism allowed).
- Hidden/system artifacts are excluded by default.
- Large media warning threshold default is 100 MB and configurable.
- UI re-scan can be triggered by any LAN user.
- Scan modes include full scan and optional quick scan.
- Breadcrumb jump-to-level behavior is required.
- Slideshow is optional and only if naturally supported by selected lightbox.
- GPS is shown as raw coordinates in V1; local map rendering is deferred.

### 2026-06-03 (Follow-up Decisions)
- Supported extensions are configurable by allowlist.
- MIME/content sniff checks are included in addition to extension filtering.
- Catalog index persistence is required; embedded local DB approach is preferred.
- Quick-scan uses file and folder modify timestamps (mtime) for delta detection.

### 2026-06-03 (Technology Direction)
- Preferred implementation language is Go.
- SQLite is selected as the recommended embedded index database for V1.
- Video preview strategy in V1 is poster-frame only.
- Alpine Linux complete native packaging support is required, but may be delivered after V1.

## 16. ADR References
- ADR-001: Go Service Baseline
- ADR-002: SQLite Index Store
- ADR-003: Quick-Scan Using mtime Delta Strategy
- ADR-004: V1 Video Preview Strategy is Poster-Frame Only
- ADR-005: Alpine Native Packaging Roadmap

## 17. Technical Specs (Draft)
- Implementation blueprint: docs/architecture/implementation-blueprint-v1.md
- OpenAPI draft: docs/api/openapi.v1.yaml
- SQLite schema draft: docs/db/sqlite-schema-v1.sql
