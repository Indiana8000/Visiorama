# ADR-002: SQLite Index Store

- Status: Accepted
- Date: 2026-06-03

## Context
The service requires persistent index/catalog data across restarts, fast browse startup, low operational overhead, and no external infrastructure dependency for home-network deployment.

## Decision
Use SQLite as the embedded persistent index store in V1.

## Rationale
- Zero external database service required.
- Mature, reliable, and easy to back up.
- Good query performance for album/media navigation with proper indexes.
- Fits single-node local deployment model.

## Consequences
- Schema design and index tuning are critical for 100k scale responsiveness.
- Long-running scan writes should use batching and transactions.
- WAL mode and pragmatic settings should be evaluated for read-heavy workloads.

## Rejected Alternatives
- External PostgreSQL/MySQL: unnecessary operational burden for initial private LAN scope.
- In-memory-only index: fails persistence requirement and restart behavior goals.
- Embedded KV-only approach: weaker ad-hoc query flexibility for browse features.

## Follow-ups
- Define normalized schema for Album, Media, ScanJob.
- Add migration strategy and integrity check tooling.
- Benchmark browse and scan query paths with realistic datasets.
