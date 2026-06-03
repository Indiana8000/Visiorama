# ADR-003: Quick-Scan Using mtime Delta Strategy

- Status: Accepted
- Date: 2026-06-03

## Context
Full scans over large datasets can be expensive. The system must support quick-scan mode while preserving correctness and allowing fallback to full scan when confidence is low.

## Decision
Implement quick-scan delta detection based on file and folder modification timestamps (mtime), with full-scan fallback.

## Rationale
- Minimal extra metadata tracking required.
- Lightweight and broadly supported filesystem signal.
- Significant scan-time reduction when changes are localized.

## Consequences
- Filesystems with coarse mtime precision may reduce delta accuracy.
- Rename/move semantics may need additional handling in specific paths.
- Quick scan must remain best-effort and trigger full scan when uncertain.

## Rejected Alternatives
- File hashing of all items each run: too costly for frequent scans at target size.
- Filesystem watcher model: explicitly out of scope for V1.
- Metadata database change journal only: less robust when source tree changes outside service control.

## Follow-ups
- Persist last-seen mtime state for folders and files.
- Define fallback heuristics and uncertainty thresholds.
- Record scan mode and fallback events in operational logs.
