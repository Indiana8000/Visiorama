# Visiorama

A private, read-only photo and video gallery service for home networks. Single self-contained binary with an embedded Vue 3 frontend. Designed for up to 100,000 media items with SQLite-backed indexing and fast thumbnail generation.

---

## Features

- **Folder-based albums** — recursive directory hierarchy reflected as nested albums
- **Image & video support** — JPG, PNG, WebP, GIF, HEIC, TIFF, AVIF, MP4, MKV, MOV, WebM, AVI, M4V
- **EXIF metadata** — capture date, resolution, camera/lens info, GPS coordinates, orientation correction
- **Thumbnail caching** — configurable multi-size thumbnails (240px, 480px, 960px)
- **Quick scan** — delta detection via mtime; only re-indexes changed files
- **Full scan** — complete rebuild of the media index
- **Lightbox viewer** — full-screen image/video view with keyboard and touch navigation
- **Natural sorting** — `item2` sorts before `item10`
- **Large media warning** — configurable threshold (default 100 MB) before playback
- **Mobile-responsive** — touch-friendly on desktop and mobile

---

## Installation (Linux — recommended)

The install script downloads the latest release binary, creates a dedicated service user, writes a default config, and registers a system service automatically.

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

1. Detects architecture (`amd64`, `arm64`, `armv7`) and downloads the matching binary to `/usr/local/bin/visiorama`
2. Verifies SHA-256 checksum before installing
3. Creates a `visiorama` system user with no login shell
4. Creates `/var/lib/visiorama/thumbs/` for thumbnail cache and the SQLite index
5. Writes a starter config to `/etc/visiorama/visiorama.yaml` (only if one doesn't already exist)
6. Registers and enables a system service (systemd or OpenRC, whichever is present)

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

3. Optionally install ffmpeg for video thumbnail generation:
   ```bash
   # Alpine
   apk add ffmpeg
   # Debian/Ubuntu
   apt install ffmpeg
   ```

4. Start the service:
   ```bash
   # systemd
   sudo systemctl start visiorama

   # OpenRC
   sudo rc-service visiorama start
   ```

5. Open `http://<host>:8080` in your browser.

**Paths installed:**

| Path | Purpose |
|------|---------|
| `/usr/local/bin/visiorama` | Binary |
| `/etc/visiorama/visiorama.yaml` | Configuration |
| `/var/lib/visiorama/index.db` | SQLite media index |
| `/var/lib/visiorama/thumbs/` | Thumbnail cache |

---

## Quick Start (build from source)

### Prerequisites

- Go 1.25+
- Node.js 20+ (for frontend build)

### Build

```bash
# Build frontend
cd web/app
npm install
npm run build
cd ../..

# Build binary
go build -o visiorama ./cmd/visiorama
```

### Configure

Copy the example config and edit it:

```bash
cp configs/visiorama.example.yaml configs/visiorama.yaml
```

Set at minimum the `library.root` path to your media directory.

### Run

```bash
./visiorama --config configs/visiorama.yaml
```

Open `http://localhost:8080` in your browser.

---

## Configuration

`configs/visiorama.yaml`:

```yaml
server:
  host: 0.0.0.0
  port: 8080

library:
  root: /path/to/your/media
  show_empty_albums: false

scan:
  mode: quick           # quick | full
  workers: 4

filtering:
  exclude_patterns:
    - ".*"
    - "@eaDir"
    - "Thumbs.db"
  extensions:           # allowed file extensions (images + videos)
  mime_sniffing: true

thumbnails:
  cache_dir: /path/to/thumb/cache
  sizes: [240, 480, 960]

limits:
  large_media_mb: 100   # warn before playing files larger than this

database:
  path: /path/to/visiorama.db
```

---

## API

The REST API is documented via OpenAPI 3.0.3 at [`docs/api/openapi.v1.yaml`](docs/api/openapi.v1.yaml).

Key endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/albums` | List root albums |
| `GET` | `/api/v1/albums/{id}` | Get album with media |
| `GET` | `/api/v1/media/{id}` | Get media metadata |
| `GET` | `/api/v1/media/{id}/thumb` | Serve thumbnail |
| `GET` | `/api/v1/media/{id}/stream` | Stream original file |
| `POST` | `/api/v1/scan` | Trigger a scan |
| `GET` | `/api/v1/health` | Health check |

---

## Architecture

```
cmd/visiorama/        Entry point — config loading, server start
internal/
  api/                HTTP handlers and routing
  app/                Bootstrap and configuration
  index/              SQLite persistence (albums, media, scans)
  scan/               File scanning, EXIF extraction, classification
  thumbs/             Thumbnail generation and caching
  stream/             Image and video streaming
  util/               MIME checking, natural sort, path safety
  observability/      Logging
web/
  embed.go            Embeds compiled frontend into the binary
  app/                Vue 3 + Vite frontend
configs/              YAML configuration
docs/                 ADRs, API spec, architecture docs
```

- **Backend**: Go 1.25, SQLite via `modernc.org/sqlite`, EXIF via `rwcarlsen/goexif`
- **Frontend**: Vue 3, Vue Router, Pinia, Vite
- **Distribution**: Single static binary with embedded frontend; no runtime dependencies

---

## Release

GitHub Actions builds and releases multi-platform binaries automatically on tag push.

Targets: `linux/amd64`, `linux/arm64`, `linux/armv7`

See [`.github/workflows/release.yml`](.github/workflows/release.yml).

---

## License

Private use. No license for redistribution.
