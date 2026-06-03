# Gallery Service Backlog (Pre-Technology)

## Priority Legend
- P0: Required for usable V1
- P1: Strongly recommended for V1 quality
- P2: Optional after V1 stabilization

## Epic A: Catalog and Album Navigation

### A-1 Root Path Configuration (P0)
- Add configurable media root path.
- Validate path readability at startup.
- Expose clear error when unavailable.
- Persist index/catalog across restarts.

Acceptance checks:
- Valid path starts normally.
- Invalid path blocks ready state with explicit message.
- Restart keeps index and does not require mandatory full re-scan before browse.

### A-2 Recursive Album Discovery (P0)
- Treat every folder as album.
- Preserve nested album hierarchy.
- Keep empty albums visible.

Acceptance checks:
- Nested levels are represented correctly.
- Empty folders are shown.

### A-3 Natural Sorting (P0)
- Apply natural-name ordering to albums and media.

Acceptance checks:
- Sample set sorts as expected: 1, 2, 10.

### A-4 Album Tile View (P0)
- Display tile with album name, random cover, and media count.
- Use recursive media count for tile display.
- Keep random cover selection performance-first (cached/pseudo-random policy acceptable).

Acceptance checks:
- Missing-cover fallback is visible.
- Count matches indexed media items in album subtree.

### A-5 Breadcrumb Navigation (P0)
- Allow direct jump to any ancestor level from breadcrumb.

Acceptance checks:
- User can navigate back to any previous level in one click/tap.

## Epic B: Media Presentation

### B-1 Thumbnail Generation and Caching (P0)
- Generate thumbnails/previews into service-owned cache.
- Keep source root untouched.
- Use poster-frame extraction for video previews in V1.

Acceptance checks:
- Thumbnail appears on first request.
- Repeat request is cache hit and faster.
- Video entries have poster frames without animated preview generation.

### B-2 Media Grid with Paging/Incremental Loading (P0)
- Render grid for current album.
- Support large albums without blocking UI.

Acceptance checks:
- Large album remains responsive.
- Initial load does not fetch all items at once.

### B-3 Lightbox (P0)
- Open image/video in full view.
- Prev/next navigation across current album list.

Acceptance checks:
- Keyboard and touch navigation work.
- Video plays in lightbox.

### B-4 Optional Slideshow Support (P2)
- Enable slideshow only if it comes naturally with chosen lightbox and low complexity.

Acceptance checks:
- Feature is either enabled with minimal implementation cost or explicitly skipped and documented.

## Epic C: Metadata and Correct Rendering

### C-1 Metadata Extraction Pipeline (P0)
- Extract filename, capture date, size, resolution, duration, camera/lens, GPS.

Acceptance checks:
- Available metadata fields are displayed.
- Missing fields do not create errors.
- GPS is displayed as raw coordinates in V1.

### C-2 EXIF Orientation Handling (P0)
- Apply orientation for correct display.

Acceptance checks:
- Rotated source image appears visually correct.

## Epic D: Refresh and Operations

### D-1 Manual Re-Scan from UI (P0)
- Add scan trigger button.
- Show scan status and result.
- Trigger is available to all LAN users.

Acceptance checks:
- Newly added files appear after scan.
- Deleted files disappear after scan.

### D-1a Scan Mode Selection (P1)
- Add scan mode selector: full and quick.
- Implement quick-scan delta based on file/folder modify timestamps.

Acceptance checks:
- User can choose full/quick in UI.
- System can fall back to full scan when quick-scan delta is uncertain.
- Quick-scan includes changes where mtime differs since last scan snapshot.

### D-2 Manual Re-Scan from CLI (P0)
- Provide server-side command to run scan.
- Support --mode full|quick.

Acceptance checks:
- CLI returns success/failure code.
- Optional wait mode reports completion.

### D-4 Exclusion Rules and Hidden/System Defaults (P0)
- Exclude hidden/system artifacts by default.
- Support configurable exclude patterns.
- Support configurable media extension allowlist.
- Add MIME/content sniff validation during scan/classification.

Acceptance checks:
- Default excludes cover dotfiles, @eaDir, and Thumbs.db.
- Additional patterns can be configured without code changes.
- Allowlist change is effective after next scan.
- Incorrect extension files are filtered using MIME/content sniff mismatch rules.

### D-3 Health and Logging Baseline (P1)
- Health endpoint with source path status.
- Structured scan logs and extraction errors.

Acceptance checks:
- Health reflects mounted/unmounted root state.
- Scan summary metrics are logged.

## Epic E: Scale Hardening (100k)

### E-1 Scan Throughput Tuning (P1)
- Bounded concurrency and memory usage.

Acceptance checks:
- Scan does not exhaust memory on large catalogs.

### E-2 Query and Listing Optimization (P1)
- Fast album open and media list retrieval.

Acceptance checks:
- Album navigation remains responsive at 100k dataset scale.

### E-5 Embedded Index Storage (P1)
- Use embedded local database persistence for index/catalog state.
- Preferred V1 implementation target: SQLite.

Acceptance checks:
- Startup can serve catalog from persisted state.
- Recovery path exists for index rebuild.

### E-3 Cache Budget Policy (P1)
- Define cache size limits and cleanup behavior.

Acceptance checks:
- Cache can be pruned without data loss.

### E-4 Large Media Warning Threshold (P1)
- Add pre-open warning for large files with configurable threshold.
- Default threshold set to 100 MB.

Acceptance checks:
- Warning appears for files above threshold.
- Threshold can be changed in config.

## Epic F: UX and Accessibility

### F-1 Mobile-First Responsiveness (P0)
- Ensure touch-friendly controls and adaptive layout.

Acceptance checks:
- Major views usable on phone and tablet.

### F-2 English-Only Copy (P0)
- All labels/messages in English.

Acceptance checks:
- No non-English user-facing strings.

### F-3 Keyboard Basics (P1)
- Lightbox close and navigation shortcuts.

Acceptance checks:
- Keyboard interactions are functional on desktop.

## Epic G: Packaging and Deployment

### G-1 Alpine Native Package (P2)
- Provide native package artifact for Alpine Linux deployment as post-V1 hardening.

Acceptance checks:
- Installation is possible on Alpine without container runtime.
- Service can run with documented system integration (service user, directories, startup).

## Epic H: Technology Baseline

### H-1 Go Service Baseline (P1)
- Implement backend service in Go.

Acceptance checks:
- Core scan/list APIs and scan jobs run in Go service process.

## Cross-Cutting Constraints
- Read-only source media behavior is mandatory.
- No auth in V1 (private LAN assumption).
- No realtime file watch in V1.

## Open Decision Queue (for next session)
- Thumbnail/video preview tooling.
- API design detail level (path-based vs id-based addressing).
- Final extension allowlist defaults.
- Alpine package format and service manager integration details.

## ADR Traceability
- ADR-001 maps to Epic H.
- ADR-002 maps to Epic E and Epic A.
- ADR-003 maps to Epic D.
- ADR-004 maps to Epic B.
- ADR-005 maps to Epic G.
