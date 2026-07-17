# Implementation Blueprint V1

## 1. Objective
Technical baseline for the Visiorama Go backend, SQLite index store, and Vue 3 frontend.

## 2. Repository Structure

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
      handlers_admin.go
      handlers_map.go
      handlers_health.go
      handlers_convert.go
      handlers_transcode.go
      dto.go
      mapping.go
      respond.go
      static.go
    scan/
      runner.go
      scanner_full.go
      scanner_quick.go
      scanner_orphan.go
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
        transcode_repo.go
    thumbs/
      generator.go
      cache.go
      video_poster.go
      placeholder.go
      warmer.go
    stream/
      image_stream.go
      video_stream.go
    transcode/
      runner.go
    convert/
      converter.go
      cache.go
    mapview/
      cluster.go
      cluster_test.go
    health/
      health.go
    server/
      server.go
      mem_linux.go
      mem_windows.go
      mem_other.go
    observability/
      logging.go
    util/
      naturalsort.go
      mimecheck.go
      pathsafe.go
  web/
    embed.go
    app/
      (Vue 3 + Vite source)
  configs/
    visiorama.example.yaml
  docs/
    api/
      openapi.v1.yaml
    adr/
      ADR-001-go-service-baseline.md
      ADR-002-sqlite-index-store.md
      ADR-003-quick-scan-mtime-strategy.md
      ADR-004-v1-video-poster-only.md  (superseded by ADR-006)
      ADR-005-distribution-install-script.md
      ADR-006-video-transcode-pipeline.md
    architecture/
      implementation-blueprint-v1.md
```

## 3. Runtime Components

### 3.1 API Layer
- Album navigation (root, by-id, by-path, by-media-ids)
- Media metadata, thumbnails, streaming
- Scan job control (trigger, status, active scan)
- Admin (reset thumbnail cache)
- Map clustering and tile proxy
- Image format conversion
- Video transcode (enqueue, status, stream)
- Health

### 3.2 Scan Layer
- **Full scan:** traverses entire media root, rebuilds/updates album and media index.
- **Quick scan:** uses folder/file mtime deltas; falls back to full when delta is uncertain.
- **Orphan scan:** removes index entries whose source files no longer exist.
- Bounded worker concurrency (`scan.maxWorkers`); background thumbnail warmer suspended during scan.

### 3.3 Index Layer
- SQLite-backed catalog (albums, media, scan_jobs, scan_errors, transcode_jobs).
- Schema managed via `migrations.go`; migrations are idempotent and run at startup.
- Table recreation migrations run inside a transaction to prevent partial-state corruption.

### 3.4 Thumbnail Layer
- On-demand thumbnail generation and size-variant caching.
- Video posters via ffmpeg poster-frame extraction.
- Background warmer pre-generates thumbnails for unwarm items.
- Foreground generation semaphore limits concurrent client-triggered generation.

### 3.5 Streaming Layer
- Range-request support for images and videos.
- Path traversal protection enforced via `util.SafeJoin`.
- Serves original files read-only from configured root.

### 3.6 Transcode Layer (ADR-006)
- User-triggered, per-video H.264/AAC MP4 re-encoding via ffmpeg.
- Queue depth: 64 in-memory; overflow jobs persisted in DB for restart pickup.
- Output cached in `transcode.cacheDir` with configurable TTL (default: 48h).
- Image format conversion (HEIC → JPEG, etc.) with in-memory cache.

### 3.7 Map Layer
- GPS coordinates indexed per media item during scan.
- Cluster API groups GPS points by zoom/bbox for MapLibre GL.
- Style and tiles proxied from openfreemap.org (10-minute cache).

## 4. Configuration Model

```yaml
server:
  host: 0.0.0.0
  port: 8080
  memLimitMiB: 0

library:
  rootPath: /mnt/media
  includeEmptyAlbums: true

scan:
  defaultMode: quick
  quickFallbackToFull: true
  maxWorkers: 0          # 0 = auto (runtime.NumCPU())
  ignoreDirMtime: false  # set true for CIFS/SMB mounts

filtering:
  excludePatterns: [".*", "@eaDir", "Thumbs.db"]
  allowedImageExtensions: ["jpg", "jpeg", "png", "webp", "gif", "heic", "tif", "tiff", "avif"]
  allowedVideoExtensions: ["mp4", "mkv", "mov", "webm", "avi", "m4v"]
  enableMimeSniff: true

thumbnails:
  cacheDir: /var/lib/visiorama/thumbs
  sizes: [320, 640]
  aspectRatioW: 4
  aspectRatioH: 3

transcode:
  cacheDir: /var/lib/visiorama/transcodes
  ttlHours: 48
  imageMaxDim: 2400

limits:
  largeMediaWarningBytes: 104857600

database:
  sqlitePath: /var/lib/visiorama/index.db
```

## 5. API Surface

See `docs/api/openapi.v1.yaml` for full spec. Key resource groups:

| Group | Prefix |
|-------|--------|
| Albums | `/api/albums/` |
| Media | `/api/media/{mediaId}/` |
| Scans | `/api/scans/` |
| Transcode | `/api/transcode-jobs/`, `/api/media/{id}/transcode` |
| Map | `/api/map/` |
| Admin | `/api/reset_thumbs` |
| Health | `/api/health` |

## 6. Performance Baseline Targets
- Catalog size: up to 100k media items.
- Album listing: p95 < 300 ms from warm index.
- Thumbnail: first request generates; subsequent requests are cache-hit fast.
- Quick scan materially faster than full scan in low-change scenarios.

## 7. Security and Safety Baseline
- Source media is read-only.
- Writes only to service-owned database/cache paths.
- Path traversal prevention mandatory for all path-based operations (`util.SafeJoin`).
- `os.RemoveAll` on cache dirs guarded by depth check before execution.
- Host header sanitized before use in URL construction.
- No auth in V1 (trusted LAN assumption).

## 8. V1 Exit Criteria
- Browse nested albums with recursive media counts.
- View images/videos in lightbox with metadata and GPS.
- Slideshow mode operational.
- Map view with GPS clustering.
- Manual re-scan via UI (full, quick, orphan modes).
- Quick scan with mtime fallback.
- Persistent index survives restart.
- Mobile-friendly UI across core screens.
- Video transcode pipeline operational (user-triggered).
