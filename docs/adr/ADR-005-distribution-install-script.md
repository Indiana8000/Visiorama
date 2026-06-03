# ADR-005: Distribution via Install Script and GitHub Releases

- Status: Accepted (supersedes ADR-005-alpine-packaging-roadmap.md)
- Date: 2026-06-04

## Context

Visiorama targets home-lab users running Raspberry Pi OS (Debian/arm64), Proxmox Alpine LXC
containers, and similar self-hosted Linux environments.  Users expect a simple installation
experience without package managers or container runtimes.

## Decision

Distribute via a shell install script (`install.sh`) that downloads a pre-built binary from
GitHub Releases and registers a system service.

## Rationale

- Single binary, no CGo, no runtime dependencies beyond optional ffmpeg.
- Cross-compilation for `linux/amd64`, `linux/arm64`, `linux/armv7` is trivial with `GOOS/GOARCH`.
- Install script auto-detects init system (systemd / OpenRC) and writes the appropriate service unit.
- One script covers all target distributions — no separate `.deb` or `.apk` pipelines needed.
- GitHub Releases provides free hosting, checksum files, and a stable `latest` API endpoint.

## Consequences

- GitHub Actions workflow builds all three target architectures on each `v*` tag push.
- Frontend (`web/app/dist/`) must be built before `go build` (handled in CI).
- `web/app/dist/` is committed to the repository so local `go build` works without Node.js.
- Config file is written once at install time and never overwritten by upgrades.
- Service runs as dedicated `visiorama` user (no shell, no login).
- ffmpeg is an optional runtime dependency; thumbnails degrade gracefully to placeholder SVG.

## Rejected Alternatives

- Alpine `.apk` native package: separate pipeline, no benefit over install script for this audience.
- Docker Compose: valid but adds Docker as a hard dependency; install script covers both audiences.
- Debian `.deb`: additional tooling, PPA hosting cost; not justified for current scale.
