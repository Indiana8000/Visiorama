# Visiorama

A private, read-only photo and video gallery service for home networks. Single self-contained binary with an embedded Vue 3 frontend. Designed for up to 100,000 media items with SQLite-backed indexing and fast thumbnail generation.

---

## Features

- **Folder-based albums** — recursive directory hierarchy reflected as nested albums
- **Image & video support** — JPG, PNG, WebP, GIF, HEIC, TIFF, AVIF, MP4, MKV, MOV, WebM, AVI, M4V
- **EXIF metadata** — capture date, resolution, camera/lens info, GPS coordinates, orientation correction
- **Thumbnail caching** — configurable multi-size thumbnails
- **On-demand transcoding** — video/image conversion for browser playback, with TTL-based cache eviction
- **Quick scan** — delta detection via mtime; only re-indexes changed files
- **Full scan** — complete rebuild of the media index
- **Orphan scan** — reconciles index rows against files removed outside of a normal scan
- **AI recognition (optional)** — face detection/clustering and object/scene labels via a separate `visiorama-ai` sidecar (ONNX Runtime); person tagging, cluster management, and per-media labels
- **Map view** — GPS-tagged media plotted on a map with clustering
- **Lightbox viewer** — full-screen image/video view with keyboard and touch navigation
- **Natural sorting** — `item2` sorts before `item10`
- **Large media warning** — configurable threshold (default 100 MB) before playback
- **Mobile-responsive** — touch-friendly on desktop and mobile

---

## Installation (Linux — recommended)

The install script downloads the latest release binary (and the optional AI sidecar, if published), creates a dedicated service user, writes a default config, and registers a system service automatically.

```bash
curl -fsSL https://raw.githubusercontent.com/Indiana8000/visiorama/main/install.sh | sudo sh
```

Or download and inspect first:

```bash
curl -fsSL https://raw.githubusercontent.com/Indiana8000/visiorama/main/install.sh -o install.sh
# review install.sh ...
sudo sh install.sh
```

**What it does:**

1. Detects architecture (`amd64`, `arm64`, `armv7`) and init system (systemd or OpenRC)
2. Downloads `visiorama` to `/usr/local/bin/visiorama`, verifying its SHA-256 checksum
3. Downloads `visiorama-ai` (the AI sidecar) the same way if it exists for the release; skips gracefully if not
4. On Alpine, installs `gcompat` and `onnxruntime` for the AI sidecar and creates the `libonnxruntime.so` symlink automatically
5. Creates a `visiorama` system user with no login shell
6. Creates `/var/lib/visiorama/{thumbs,transcodes,models,crops}/` for caches, AI models, and the SQLite index
7. Writes a starter config to `/etc/visiorama/visiorama.yaml` (only if one doesn't already exist)
8. Registers and enables system service unit(s): `visiorama`, plus `visiorama-ai` when the AI sidecar was installed

**After installation:**

1. Edit the config and set your media path:
   ```bash
   sudo nano /etc/visiorama/visiorama.yaml
   # set library.rootPath to your photo directory
   ```

2. If your photos are on a mounted drive, grant the service user access:
   ```bash
   sudo usermod -aG <mountgroup> visiorama
   ```

3. Optionally install ffmpeg (video thumbnails/transcoding) and ImageMagick (HEIC/AVIF/TIFF support):
   ```bash
   # Alpine
   apk add ffmpeg imagemagick imagemagick-heic
   # Debian/Ubuntu
   apt install ffmpeg imagemagick libheif1
   ```

4. If the AI sidecar was installed, ONNX Runtime is required — the installer sets this up on Alpine automatically; on other distros, install it manually or set `ORT_LIB_PATH`. Models (~300 MB) download automatically on first start.

5. Start the service:
   ```bash
   # systemd
   sudo systemctl start visiorama-ai visiorama   # omit visiorama-ai if AI sidecar not installed

   # OpenRC
   sudo rc-service visiorama-ai start && sudo rc-service visiorama start
   ```

6. Open `http://<host>:8080` in your browser.

**Paths installed:**

| Path | Purpose |
|------|---------|
| `/usr/local/bin/visiorama` | Main server binary |
| `/usr/local/bin/visiorama-ai` | AI inference sidecar binary (optional) |
| `/etc/visiorama/visiorama.yaml` | Configuration |
| `/var/lib/visiorama/index.db` | SQLite media index |
| `/var/lib/visiorama/thumbs/` | Thumbnail cache |
| `/var/lib/visiorama/transcodes/` | Transcoded media cache |
| `/var/lib/visiorama/models/` | AI models (ONNX) |
| `/var/lib/visiorama/crops/` | AI face crop thumbnails |
| `/etc/systemd/system/visiorama.service` or `/etc/init.d/visiorama` | Service unit |
| `/etc/systemd/system/visiorama-ai.service` or `/etc/init.d/visiorama-ai` | AI sidecar service unit (optional) |

---

## Uninstalling

There's no uninstall script yet — remove the installed pieces manually:

```bash
# 1. Stop and disable the service(s)
# systemd:
sudo systemctl stop visiorama visiorama-ai
sudo systemctl disable visiorama visiorama-ai
sudo rm -f /etc/systemd/system/visiorama.service /etc/systemd/system/visiorama-ai.service
sudo systemctl daemon-reload

# OpenRC:
sudo rc-service visiorama stop
sudo rc-service visiorama-ai stop
sudo rc-update del visiorama default
sudo rc-update del visiorama-ai default
sudo rm -f /etc/init.d/visiorama /etc/init.d/visiorama-ai

# 2. Remove the binaries
sudo rm -f /usr/local/bin/visiorama /usr/local/bin/visiorama-ai

# 3. Remove data (index, thumbnails, transcodes, AI models/crops) — irreversible
sudo rm -rf /var/lib/visiorama

# 4. Remove the config — keep this if you plan to reinstall later
sudo rm -rf /etc/visiorama

# 5. Remove the service user
sudo userdel visiorama       # Debian/Ubuntu
sudo deluser visiorama       # Alpine

# 6. Optional: remove ffmpeg/ImageMagick/ONNX Runtime if installed solely for visiorama
```

Skip step 3 (and back up first) if you only want to reinstall or upgrade — the install script never overwrites an existing config, and reusing the same data directory preserves your index and thumbnail cache.

---

## Quick Start (build from source)

### Prerequisites

- Go 1.25+
- Node.js 20+ (for frontend build)
- CGO + ONNX Runtime headers (only if building `visiorama-ai`)

### Build

```bash
# Build frontend
cd web/app
npm install
npm run build
cd ../..

# Build main binary
go build -o visiorama ./cmd/visiorama
```

### Configure

Copy the example config and edit it:

```bash
cp configs/visiorama.example.yaml configs/visiorama.yaml
```

Set at minimum `library.rootPath` to your media directory.

### Run

```bash
./visiorama -config configs/visiorama.yaml
```

Open `http://localhost:8080` in your browser.

### CLI scan (without starting the server)

```bash
./visiorama scan -config configs/visiorama.yaml -mode quick   # or: full, orphan
```

---

## Configuration

`configs/visiorama.yaml` (see [`configs/visiorama.example.yaml`](configs/visiorama.example.yaml) for the full annotated version):

```yaml
server:
  host: 0.0.0.0
  port: 8080
  memLimitMiB: 0

library:
  rootPath: /path/to/your/media
  includeEmptyAlbums: true

scan:
  defaultMode: quick        # quick | full
  quickFallbackToFull: true
  ignoreDirMtime: false     # enable for CIFS/SMB shares
  maxWorkers: 0

filtering:
  excludePatterns: [".*", "@eaDir", "Thumbs.db", "#recycle"]
  allowedImageExtensions: ["jpg", "jpeg", "png", "webp", "gif", "heic", "tif", "tiff", "avif"]
  allowedVideoExtensions: ["mp4", "mkv", "mov", "webm", "avi", "m4v"]
  enableMimeSniff: true

thumbnails:
  cacheDir: /path/to/thumb/cache
  sizes: [320, 640]
  aspectRatioW: 4
  aspectRatioH: 3
  maxCacheMiB: 0

transcode:
  cacheDir: /path/to/transcode/cache
  ttlHours: 48
  imageMaxDim: 2400
  maxCacheMiB: 0

limits:
  largeMediaWarningBytes: 104857600

database:
  sqlitePath: /path/to/visiorama.db

ai:
  binary: ""            # path to visiorama-ai binary; empty = auto-detect from PATH
  socketPath: ""         # empty = /tmp/visiorama-ai.sock
  modelDir: /path/to/models
  faceCacheDir: /path/to/crops
  workers: 0
  labelMinConfidence: 0.6
  faceMinPixels: 40
  reanalyzeOnFullScan: false
  analyzeTimeout: 0
```

---

## API

The REST API is documented via OpenAPI 3.0.3 at [`docs/api/openapi.v1.yaml`](docs/api/openapi.v1.yaml).

Key endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/albums/root` | List root albums |
| `GET` | `/api/albums/{albumId}` | Get album with media |
| `GET` | `/api/media/{mediaId}/metadata` | Get media metadata |
| `GET` | `/api/media/{mediaId}/thumbnail` | Serve thumbnail |
| `GET` | `/api/media/{mediaId}/stream` | Stream original file |
| `POST` | `/api/scans` | Trigger a scan |
| `GET` | `/api/ai/status` | AI sidecar status |
| `GET` | `/api/ai/persons` | List tagged persons (face recognition) |
| `GET` | `/api/map/clusters` | GPS clusters for map view |
| `GET` | `/api/health` | Health check |

---

## Architecture

```
cmd/visiorama/        Entry point — config loading, server start, CLI scan command
internal/
  ai/                 AI sidecar client (face/object recognition over Unix socket)
  api/                HTTP handlers and routing
  app/                Bootstrap and configuration
  cache/              Generic disk cache with TTL/size-based eviction
  convert/            On-demand image format conversion
  index/              SQLite persistence (albums, media, scans, AI data, persons)
  mapview/            GPS clustering and map tile/style proxying
  observability/      Logging
  scan/               File scanning, EXIF extraction, classification
  server/             HTTP server wiring and lifecycle
  thumbs/             Thumbnail generation and caching
  transcode/          Video/image transcoding for browser playback
  util/               MIME checking, natural sort, path safety
web/
  embed.go            Embeds compiled frontend into the binary
  app/                Vue 3 + Vite frontend
configs/              YAML configuration
docs/                 ADRs, API spec, architecture docs
```

- **Backend**: Go 1.25, SQLite via `modernc.org/sqlite`, EXIF via `rwcarlsen/goexif`
- **AI sidecar** (`visiorama-ai`, optional): separate CGO binary using ONNX Runtime for face/object recognition, communicating over a Unix socket
- **Frontend**: Vue 3, Vue Router, Pinia, Vite
- **Distribution**: Single static binary with embedded frontend; no runtime dependencies beyond optional ffmpeg/ImageMagick/ONNX Runtime

---

## Release

GitHub Actions builds and releases multi-platform binaries automatically on tag push.

Targets: `linux/amd64`, `linux/arm64`, `linux/armv7`

See [`.github/workflows/release.yml`](.github/workflows/release.yml).

---

## License

Private use. No license for redistribution.
