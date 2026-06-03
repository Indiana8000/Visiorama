# Implementation Blueprint V1

## 1. Objective
Translate the approved planning and ADR set into an implementation-ready technical baseline for a Go backend, SQLite index store, and web gallery frontend.

## 2. Proposed Repository Structure

```text
visiorama/
  cmd/
    visiorama/
      main.go
  internal/
    app/
      bootstrap.go
      config.go
    api/
      router.go
      handlers_albums.go
      handlers_media.go
      handlers_scan.go
      dto.go
    scan/
      scanner_full.go
      scanner_quick.go
      mtime_delta.go
      classifier.go
      exif_video_extract.go
    index/
      store.go
      migrations.go
      repositories/
        albums_repo.go
        media_repo.go
        scan_repo.go
    thumbs/
      generator.go
      cache.go
      video_poster.go
    stream/
      image_stream.go
      video_stream.go
    health/
      health.go
    observability/
      logging.go
      metrics.go
    util/
      naturalsort.go
      mimecheck.go
      pathsafe.go
  web/
    app/
      (frontend source)
    dist/
      (built assets served by backend)
  configs/
    visiorama.example.yaml
  scripts/
    scan.sh
  docs/
    api/
      openapi.v1.yaml
    db/
      sqlite-schema-v1.sql
    adr/
      ADR-001-go-service-baseline.md
      ADR-002-sqlite-index-store.md
      ADR-003-quick-scan-mtime-strategy.md
      ADR-004-v1-video-poster-only.md
      ADR-005-alpine-packaging-roadmap.md
```

## 3. Runtime Components

### 3.1 API Layer
- Serves album navigation, media metadata, media streams, thumbnails, and scan job control.
- Exposes health and scan status endpoints.

### 3.2 Scan Layer
- Full scan:
  - Traverses full media root.
  - Rebuilds/updates album and media index.
- Quick scan:
  - Uses folder/file mtime deltas.
  - Falls back to full scan when uncertainty rules trigger.

### 3.3 Index Layer
- SQLite-backed catalog persistence.
- Stores albums, media metadata, scan jobs, and scan errors.

### 3.4 Thumbnail Layer
- On-demand thumbnail generation and caching.
- Video previews via poster-frame extraction only in V1.

### 3.5 Streaming Layer
- Read-only serving of original images/videos from configured root.
- Must enforce root-path boundaries and path traversal protection.

## 4. Configuration Model (Draft)

```yaml
server:
  host: 0.0.0.0
  port: 8080

library:
  rootPath: /mnt/media
  includeEmptyAlbums: true

scan:
  defaultMode: quick
  quickFallbackToFull: true
  maxWorkers: 8

filtering:
  excludePatterns:
    - ".*"
    - "@eaDir"
    - "Thumbs.db"
  allowedImageExtensions: ["jpg", "jpeg", "png", "webp", "gif", "heic", "tif", "tiff", "avif"]
  allowedVideoExtensions: ["mp4", "mkv", "mov", "webm", "avi", "m4v"]
  enableMimeSniff: true

thumbnails:
  cacheDir: /var/lib/visiorama/thumbs
  sizes: [240, 480, 960]

limits:
  largeMediaWarningBytes: 104857600

database:
  sqlitePath: /var/lib/visiorama/index.db
```

## 5. API Strategy
- Primary resource model is ID-based for stable paging and lightweight URLs.
- Album path lookup endpoint is provided for UI routing convenience.
- Scan API supports mode selection: full or quick.

## 6. Performance Baseline Targets
- Catalog size: up to 100k media items.
- Album listing latency target:
  - p95 < 300 ms from warm index for typical pages.
- First thumbnail generation can be slower, repeated requests should be cache-hit fast.
- Quick scan should materially outperform full scan in low-change scenarios.

## 7. Security and Safety Baseline
- Source media is read-only.
- Writes allowed only to service-owned database/cache/log paths.
- Path traversal prevention is mandatory for all path-based operations.
- No auth in V1 (trusted LAN assumption).

## 8. V1 Exit Criteria
- Browse nested albums with recursive media counts.
- View images/videos in lightbox with metadata.
- Manual re-scan via UI and CLI.
- Quick scan operational with fallback behavior.
- Persistent index survives restart.
- Mobile-friendly UI behavior across core screens.
