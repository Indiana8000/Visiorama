# ADR-004: V1 Video Preview Strategy is Poster-Frame Only

- Status: Superseded by ADR-006
- Date: 2026-06-03

## Context
Video previews improve browsing but can add CPU, IO, and storage cost. V1 prioritizes lightweight performance and predictable behavior over advanced preview effects.

## Decision
Use static poster-frame generation for video previews in V1. Do not implement animated previews in V1.

## Rationale
- Lower processing overhead during scan and cache generation.
- Smaller cache footprint and simpler rendering path.
- Better fit for mobile and LAN browsing performance targets.

## Consequences
- Visual richness is lower than animated previews.
- Future migration path needed if richer previews are required.

## Rejected Alternatives
- Animated GIF/WebP preview strips: higher generation cost and larger storage use.
- On-hover transcoding: high runtime overhead and inconsistent UX.

## Superseded By
ADR-006 introduced an opt-in video transcode pipeline (libx264/AAC to MP4) that enables
browser-native playback for formats not supported by the browser. Poster-frame extraction
remains the default thumbnail strategy; transcode is user-triggered per video item.
