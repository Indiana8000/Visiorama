# ADR-006: On-Demand Video Transcode Pipeline

- Status: Accepted
- Date: 2026-07-17
- Supersedes: ADR-004 (partially)

## Context
Several common video formats (MKV, MOV, AVI, M4V) are not natively playable in all browsers.
ADR-004 deferred this problem to post-V1. During implementation it became clear that a
user-triggered transcode option is necessary for a usable gallery.

## Decision
Add an opt-in, on-demand transcode pipeline that re-encodes videos to H.264/AAC in an MP4
container using ffmpeg. Transcode is user-triggered per media item, not automatic.

## Implementation
- `internal/transcode/runner.go` — worker queue, ffmpeg invocation, output persistence
- `internal/api/handlers_transcode.go` — REST endpoints
- `internal/index/repositories/transcode_repo.go` — job persistence in SQLite
- `internal/api/handlers_convert.go` — image format conversion (HEIC → JPEG, etc.)
- `internal/convert/` — in-memory cache for converted image bytes

## API Surface
- `POST /api/media/{mediaId}/transcode` — enqueue transcode job, returns `{ jobId }`
- `GET /api/transcode-jobs/{jobId}` — poll job status (queued/running/success/failed)
- `GET /api/media/{mediaId}/transcode/stream` — stream transcoded MP4 output
- `GET /api/media/{mediaId}/convert` — on-demand image format conversion (JPEG stream)

## Rationale
- Poster-frame extraction (ADR-004) remains the default and covers the common case.
- Transcode is opt-in to avoid background CPU/storage cost for already-compatible formats.
- ffmpeg covers all target input formats without additional library dependencies.
- Output files are stored in a service-owned cache directory with configurable TTL.

## Consequences
- ffmpeg must be present in the deployment environment.
- Transcoded files consume additional disk space; TTL cleanup runs hourly.
- Queue depth is 64 jobs in-memory; overflow is persisted in DB for next restart pickup.

## Rejected Alternatives
- Automatic transcode on scan: too much background cost, blocks scans unnecessarily.
- Browser-side transcoding via WebAssembly: too slow and memory-intensive for large files.
