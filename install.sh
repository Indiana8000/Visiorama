#!/bin/sh
set -e
set -o pipefail 2>/dev/null || true  # pipefail where supported (not busybox ash)

REPO="Indiana8000/visiorama"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/visiorama"
DATA_DIR="/var/lib/visiorama"
SERVICE_USER="visiorama"

# ── detect architecture ────────────────────────────────────────────────────────
detect_arch() {
  case "$(uname -m)" in
    x86_64)  echo "amd64" ;;
    aarch64) echo "arm64" ;;
    armv7l)  echo "armv7" ;;
    *)
      echo "Unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

# ── detect init system ─────────────────────────────────────────────────────────
detect_init() {
  if [ -f /sbin/openrc ] || [ -f /sbin/rc-service ]; then
    echo "openrc"
  elif [ -d /run/systemd/system ]; then
    echo "systemd"
  else
    echo "none"
  fi
}

# ── detect Linux distro ───────────────────────────────────────────────────────
detect_distro() {
  if grep -q "Alpine" /etc/*release 2>/dev/null; then
    echo "alpine"
  elif grep -q "Debian" /etc/*release 2>/dev/null || grep -q "Ubuntu" /etc/*release 2>/dev/null; then
    echo "debian"
  else
    echo ""
  fi
}

# ── resolve latest release tag ────────────────────────────────────────────────
latest_tag() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
}

# ── download + verify a file ──────────────────────────────────────────────────
download_verified() {
  local url="$1" dest="$2" label="$3"
  local checksumUrl="${url}.sha256"
  local tmp
  tmp=$(mktemp)

  echo "  Downloading ${label} from ${url}"
  curl -fsSL -o "${tmp}" "${url}" || { echo "${label} download failed." >&2; rm -f "${tmp}"; exit 1; }

  local expected
  expected=$(curl -fsSL "${checksumUrl}" 2>/dev/null | awk '{print $1}')
  if [ -n "${expected}" ]; then
    local actual
    actual=$(sha256sum "${tmp}" | awk '{print $1}')
    if [ "${expected}" != "${actual}" ]; then
      echo "Checksum mismatch for ${label}!" >&2
      rm -f "${tmp}"
      exit 1
    fi
  else
    echo "  Warning: no checksum available for ${label} — skipping verification"
  fi

  install -m 755 "${tmp}" "${dest}"
  rm -f "${tmp}"
}

# ── install systemd units ─────────────────────────────────────────────────────
install_systemd() {
  # visiorama-ai sidecar (socket-activated companion)
  cat > /etc/systemd/system/visiorama-ai.service <<EOF
[Unit]
Description=Visiorama AI inference sidecar
After=network.target

[Service]
User=${SERVICE_USER}
Environment=ORT_LIB_PATH=/usr/lib/libonnxruntime.so
ExecStart=${INSTALL_DIR}/visiorama-ai \
  -socket /run/visiorama/visiorama-ai.sock \
  -models ${DATA_DIR}/models \
  -crops  ${DATA_DIR}/crops
Restart=on-failure
RestartSec=5
RuntimeDirectory=visiorama

[Install]
WantedBy=multi-user.target
EOF

  # main visiorama server
  cat > /etc/systemd/system/visiorama.service <<EOF
[Unit]
Description=Visiorama photo gallery
After=network.target visiorama-ai.service

[Service]
User=${SERVICE_USER}
ExecStart=${INSTALL_DIR}/visiorama -config ${CONFIG_DIR}/visiorama.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable visiorama-ai visiorama
  echo "  systemd units installed."
  echo "  Start with: systemctl start visiorama-ai visiorama"
}

# ── install openrc services ───────────────────────────────────────────────────
install_openrc() {
  # visiorama-ai sidecar
  cat > /etc/init.d/visiorama-ai <<EOF
#!/sbin/openrc-run

name="visiorama-ai"
description="Visiorama AI inference sidecar"
command="${INSTALL_DIR}/visiorama-ai"
command_args="-socket /run/visiorama/visiorama-ai.sock -models ${DATA_DIR}/models -crops ${DATA_DIR}/crops"
command_user="${SERVICE_USER}"
pidfile="/run/visiorama-ai.pid"
command_background=true
output_log="/var/log/visiorama-ai.log"
error_log="/var/log/visiorama-ai.log"
export ORT_LIB_PATH=/usr/lib/libonnxruntime.so

depend() {
  need net
}

start_pre() {
  mkdir -p /run/visiorama
  chown ${SERVICE_USER} /run/visiorama
}
EOF
  chmod +x /etc/init.d/visiorama-ai

  # main server
  cat > /etc/init.d/visiorama <<EOF
#!/sbin/openrc-run

name="visiorama"
description="Visiorama photo gallery"
command="${INSTALL_DIR}/visiorama"
command_args="-config ${CONFIG_DIR}/visiorama.yaml"
command_user="${SERVICE_USER}"
pidfile="/run/visiorama.pid"
command_background=true
output_log="/var/log/visiorama.log"
error_log="/var/log/visiorama.log"

depend() {
  need net
  after visiorama-ai
}
EOF
  chmod +x /etc/init.d/visiorama

  rc-update add visiorama-ai default
  rc-update add visiorama default
  echo "  OpenRC services installed."
  echo "  Start with: rc-service visiorama-ai start && rc-service visiorama start"
}

# ── main ──────────────────────────────────────────────────────────────────────
main() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "Run as root (or with sudo)." >&2
    exit 1
  fi

  ARCH=$(detect_arch)
  INIT=$(detect_init)
  DISTRO=$(detect_distro)
  TAG=$(latest_tag)

  if [ -z "${TAG}" ]; then
    echo "Could not resolve latest release tag. Check that a GitHub release exists for ${REPO}." >&2
    exit 1
  fi

  echo "Installing visiorama ${TAG} (${ARCH}, init=${INIT})"

  # Download main binary
  BINARY_URL="https://github.com/${REPO}/releases/download/${TAG}/visiorama-linux-${ARCH}"
  download_verified "${BINARY_URL}" "${INSTALL_DIR}/visiorama" "visiorama"
  echo "  Binary installed to ${INSTALL_DIR}/visiorama"

  # Download AI sidecar binary (CGO build with ONNX support)
  AI_BINARY_URL="https://github.com/${REPO}/releases/download/${TAG}/visiorama-ai-linux-${ARCH}"
  if curl -fsSL --head "${AI_BINARY_URL}" 2>/dev/null | grep -q "200"; then
    download_verified "${AI_BINARY_URL}" "${INSTALL_DIR}/visiorama-ai" "visiorama-ai"
    echo "  AI sidecar installed to ${INSTALL_DIR}/visiorama-ai"
    AI_AVAILABLE=true
  else
    echo "  visiorama-ai not found in release — AI features will be unavailable"
    echo "  (build from source with CGO enabled to get AI support)"
    AI_AVAILABLE=false
  fi

  # Create service group and user
  if ! id "${SERVICE_USER}" >/dev/null 2>&1; then
    if command -v addgroup >/dev/null 2>&1 && command -v adduser >/dev/null 2>&1; then
      # Alpine / BusyBox
      addgroup -S "${SERVICE_USER}" 2>/dev/null || true
      adduser -S -G "${SERVICE_USER}" -s /sbin/nologin "${SERVICE_USER}"
    else
      useradd -r -s /sbin/nologin "${SERVICE_USER}"
    fi
    echo "  Service user '${SERVICE_USER}' created"
  fi

  # Create data directories
  mkdir -p \
    "${DATA_DIR}/thumbs" \
    "${DATA_DIR}/transcodes" \
    "${DATA_DIR}/models" \
    "${DATA_DIR}/crops"
  chown -R "${SERVICE_USER}:${SERVICE_USER}" "${DATA_DIR}"
  echo "  Data directory: ${DATA_DIR}"

  # Write example config (never overwrite existing)
  mkdir -p "${CONFIG_DIR}"
  if [ ! -f "${CONFIG_DIR}/visiorama.yaml" ]; then
    cat > "${CONFIG_DIR}/visiorama.yaml" <<EOF
server:
  host: 0.0.0.0        # interface to bind; use 127.0.0.1 to restrict to localhost only
  port: 8080           # TCP port the web UI and API are served on
  memLimitMiB: 0       # Go heap limit in MiB; 0 = auto (90% of physical/cgroup RAM)

library:
  rootPath: /mnt/photos      # absolute path to your photo library root (required)
  includeEmptyAlbums: true   # show albums that contain no directly-scanned media

scan:
  defaultMode: quick         # scan mode used when triggered from the UI: "quick" or "full"
  quickFallbackToFull: true  # fall back to full scan when quick scan detects deleted directories
  ignoreDirMtime: false      # ignore directory mtime for change detection; enable for CIFS/SMB shares
  maxWorkers: 0              # concurrent media processing workers; 0 = auto (min of CPU count and RAM/512 MiB)

filtering:
  excludePatterns: [".*", "@eaDir", "Thumbs.db", "#recycle"]            # glob patterns for files/dirs to skip
  allowedImageExtensions: ["jpg", "jpeg", "png", "webp", "gif", "heic", "tif", "tiff", "avif"]  # image formats to index
  allowedVideoExtensions: ["mp4", "mkv", "mov", "webm", "avi", "m4v"]  # video formats to index
  enableMimeSniff: true      # verify file type by magic bytes in addition to extension

thumbnails:
  cacheDir: ${DATA_DIR}/thumbs  # directory for generated thumbnail files (required)
  sizes: [320, 640]          # thumbnail widths to generate in pixels; first entry is the default
  aspectRatioW: 4            # thumbnail crop aspect ratio — width component
  aspectRatioH: 3            # thumbnail crop aspect ratio — height component

transcode:
  cacheDir: ${DATA_DIR}/transcodes  # directory for transcoded video files
  ttlHours: 48               # delete transcoded files after this many hours (0 = use default 48)
  imageMaxDim: 2400          # max dimension in pixels for on-demand image conversion to JPEG

limits:
  largeMediaWarningBytes: 104857600  # log a warning when a media file exceeds this size (default 100 MiB; 0 = disable)

database:
  sqlitePath: ${DATA_DIR}/index.db  # path to the SQLite index database file (required)

ai:
  binary: ${INSTALL_DIR}/visiorama-ai  # path to visiorama-ai binary; empty = auto-detect from PATH
  socketPath: /run/visiorama/visiorama-ai.sock  # Unix socket for sidecar communication
  modelDir: ${DATA_DIR}/models    # directory where ONNX models are stored and downloaded to (~300 MB total)
  faceCacheDir: ${DATA_DIR}/crops # directory for face crop JPEG thumbnails
  workers: 0                   # concurrent inference workers inside the sidecar; 0 = auto (min 2)
  labelMinConfidence: 0.6      # minimum detection confidence to store a label (0.0–1.0)
  faceMinPixels: 40            # minimum face bounding-box size in pixels; smaller faces are ignored
  reanalyzeOnFullScan: false   # re-queue all media for AI analysis on every full scan (slow; off by default)

EOF
    echo "  Config written to ${CONFIG_DIR}/visiorama.yaml"
    echo ""
    echo "  !! Edit ${CONFIG_DIR}/visiorama.yaml and set library.rootPath before starting !!"
  else
    echo "  Config already exists at ${CONFIG_DIR}/visiorama.yaml — not overwritten"
  fi

  # Install service(s)
  case "${INIT}" in
    systemd) install_systemd ;;
    openrc)  install_openrc ;;
    *)
      echo "  No supported init system detected — skipping service registration"
      echo "  Run manually:"
      if [ "${AI_AVAILABLE}" = "true" ]; then
        echo "    ${INSTALL_DIR}/visiorama-ai -socket /run/visiorama/visiorama-ai.sock -models ${DATA_DIR}/models -crops ${DATA_DIR}/crops &"
      fi
      echo "    ${INSTALL_DIR}/visiorama -config ${CONFIG_DIR}/visiorama.yaml"
      ;;
  esac

  echo ""
  echo "Installation complete."
  echo ""
  echo "If your photo library is on a mounted drive, grant access:"
  echo "  usermod -aG <mountgroup> ${SERVICE_USER}"
  echo ""
  echo "Optional dependencies:"
  echo ""
  echo "  ffmpeg — video thumbnails + video transcoding:"
  if [ "${DISTRO}" = "alpine" ]; then
    echo "    Alpine:  apk add ffmpeg"
  elif [ "${DISTRO}" = "debian" ]; then
    echo "    Debian:  apt install ffmpeg"
  else
    echo "    Alpine:  apk add ffmpeg"
    echo "    Debian:  apt install ffmpeg"
  fi
  echo ""
  echo "  ImageMagick — HEIC/AVIF/TIFF image support (recommended):"
  if [ "${DISTRO}" = "alpine" ]; then
    echo "    Alpine:  apk add imagemagick imagemagick-heic"
  elif [ "${DISTRO}" = "debian" ]; then
    echo "    Debian:  apt install imagemagick libheif1"
    echo "    Note: libheif enables HEIC/HEIF decoding in ImageMagick."
    echo "          Without it, visiorama falls back to ffmpeg for those formats."
  else
    echo "    Alpine:  apk add imagemagick imagemagick-heic"
    echo "    Debian:  apt install imagemagick libheif1"
  fi
  echo ""
  if [ "${AI_AVAILABLE}" = "true" ]; then
    echo "  ONNX Runtime — required for AI face/object recognition:"
    if [ "${DISTRO}" = "alpine" ]; then
      echo "    Alpine:  apk add onnxruntime"
      echo "    Or set ORT_LIB_PATH to your libonnxruntime.so location."
    elif [ "${DISTRO}" = "debian" ]; then
      echo "    Debian:  apt install libonnxruntime  (if available) or download from:"
      echo "             https://github.com/microsoft/onnxruntime/releases"
      echo "    Or set ORT_LIB_PATH to your libonnxruntime.so location."
    else
      echo "    Download from: https://github.com/microsoft/onnxruntime/releases"
      echo "    Or set ORT_LIB_PATH to your libonnxruntime.so location."
    fi
    echo "    ONNX models (~300 MB) are downloaded automatically on first start."
    echo ""
  fi
}

main "$@"
