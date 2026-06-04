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

# ── resolve latest release tag ────────────────────────────────────────────────
latest_tag() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
}

# ── install systemd unit ──────────────────────────────────────────────────────
install_systemd() {
  cat > /etc/systemd/system/visiorama.service <<EOF
[Unit]
Description=Visiorama photo gallery
After=network.target

[Service]
User=${SERVICE_USER}
ExecStart=${INSTALL_DIR}/visiorama -config ${CONFIG_DIR}/visiorama.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable visiorama
  echo "  systemd unit installed. Start with: systemctl start visiorama"
}

# ── install openrc service ────────────────────────────────────────────────────
install_openrc() {
  cat > /etc/init.d/visiorama <<EOF
#!/sbin/openrc-run

name="visiorama"
description="Visiorama photo gallery"
command="${INSTALL_DIR}/visiorama"
command_args="-config ${CONFIG_DIR}/visiorama.yaml"
command_user="${SERVICE_USER}"
pidfile="/run/visiorama.pid"
command_background=true

depend() {
  need net
}
EOF
  chmod +x /etc/init.d/visiorama
  rc-update add visiorama default
  echo "  OpenRC service installed. Start with: rc-service visiorama start"
}

# ── main ──────────────────────────────────────────────────────────────────────
main() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "Run as root (or with sudo)." >&2
    exit 1
  fi

  ARCH=$(detect_arch)
  INIT=$(detect_init)
  TAG=$(latest_tag)

  if [ -z "${TAG}" ]; then
    echo "Could not resolve latest release tag. Check that a GitHub release exists for ${REPO}." >&2
    exit 1
  fi

  echo "Installing visiorama ${TAG} (${ARCH}, init=${INIT})"

  # Download binary
  BINARY_URL="https://github.com/${REPO}/releases/download/${TAG}/visiorama-linux-${ARCH}"
  CHECKSUM_URL="${BINARY_URL}.sha256"

  TMP=$(mktemp)
  echo "  Downloading ${BINARY_URL}"
  curl -fsSL -o "${TMP}" "${BINARY_URL}" || { echo "Binary download failed." >&2; rm -f "${TMP}"; exit 1; }
  EXPECTED=$(curl -fsSL "${CHECKSUM_URL}" | awk '{print $1}')
  if [ -z "${EXPECTED}" ]; then
    echo "Checksum download failed: ${CHECKSUM_URL}" >&2
    rm -f "${TMP}"
    exit 1
  fi
  ACTUAL=$(sha256sum "${TMP}" | awk '{print $1}')
  if [ "${EXPECTED}" != "${ACTUAL}" ]; then
    echo "Checksum mismatch!" >&2
    rm -f "${TMP}"
    exit 1
  fi

  install -m 755 "${TMP}" "${INSTALL_DIR}/visiorama"
  rm -f "${TMP}"
  echo "  Binary installed to ${INSTALL_DIR}/visiorama"

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

  # Create directories
  mkdir -p "${DATA_DIR}/thumbs"
  chown -R "${SERVICE_USER}:${SERVICE_USER}" "${DATA_DIR}"
  echo "  Data directory: ${DATA_DIR}"

  # Write example config (never overwrite existing)
  mkdir -p "${CONFIG_DIR}"
  if [ ! -f "${CONFIG_DIR}/visiorama.yaml" ]; then
    cat > "${CONFIG_DIR}/visiorama.yaml" <<EOF
server:
  host: 0.0.0.0
  port: 8080

library:
  rootPath: /mnt/photos
  includeEmptyAlbums: true

scan:
  defaultMode: quick
  quickFallbackToFull: true
  maxWorkers: 4

filtering:
  excludePatterns: [".*", "@eaDir", "Thumbs.db", "#recycle"]
  allowedImageExtensions: ["jpg", "jpeg", "png", "webp", "gif", "heic", "tif", "tiff", "avif"]
  allowedVideoExtensions: ["mp4", "mkv", "mov", "webm", "avi", "m4v"]
  enableMimeSniff: true

thumbnails:
  cacheDir: /var/lib/visiorama/thumbs
  sizes: [320, 640]
  aspectRatioW: 4
  aspectRatioH: 3

limits:
  largeMediaWarningBytes: 104857600

database:
  sqlitePath: ${DATA_DIR}/index.db
EOF
    echo "  Config written to ${CONFIG_DIR}/visiorama.yaml"
    echo ""
    echo "  !! Edit ${CONFIG_DIR}/visiorama.yaml and set library.rootPath before starting !!"
  else
    echo "  Config already exists at ${CONFIG_DIR}/visiorama.yaml — not overwritten"
  fi

  # Install service
  case "${INIT}" in
    systemd) install_systemd ;;
    openrc)  install_openrc ;;
    *)
      echo "  No supported init system detected — skipping service registration"
      echo "  Run manually: ${INSTALL_DIR}/visiorama -config ${CONFIG_DIR}/visiorama.yaml"
      ;;
  esac

  echo ""
  echo "Installation complete."
  echo ""
  echo "If your photo library is on a mounted drive, grant access:"
  echo "  usermod -aG <mountgroup> ${SERVICE_USER}"
  echo ""
  echo "If ffmpeg is available, video thumbnails will be generated automatically."
  echo "Install: apk add ffmpeg  /  apt install ffmpeg"
}

main "$@"
