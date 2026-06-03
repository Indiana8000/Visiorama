# ADR-001: Go Service Baseline

- Status: Accepted
- Date: 2026-06-03

## Context
The gallery service must run reliably in a home-network environment, process up to 100k media items, expose HTTP APIs, handle manual scans, and operate with low maintenance. The deployment target is Alpine Linux, and source media access is read-only.

## Decision
Use Go as the primary backend implementation language for V1.

## Rationale
- Single static binary deployment fits lightweight server operations.
- Strong concurrency primitives support parallel scanning and thumbnail workflows.
- Good cross-compilation and Linux support, including musl-compatible targets.
- Predictable memory and runtime behavior for long-running services.

## Consequences
- Team codebase standards should align around Go tooling and project structure.
- Build pipeline must produce Linux Alpine-compatible artifacts.
- Language-specific libraries for metadata extraction and media probing must be validated early.

## Rejected Alternatives
- Node.js service baseline: strong ecosystem but larger runtime footprint and process overhead for this use case.
- Python service baseline: fast iteration but weaker fit for a single lightweight production binary.
- Rust service baseline: very strong performance but higher implementation complexity for initial delivery speed.

## Follow-ups
- Define module layout and coding standards.
- Lock runtime and build targets for Alpine packaging flow.
