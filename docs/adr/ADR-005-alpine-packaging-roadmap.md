# ADR-005: Alpine Native Packaging Roadmap

- Status: Accepted
- Date: 2026-06-03

## Context
Target operations require native support on Alpine Linux. Full packaging and service integration are required, but can be delivered after V1 functional completion.

## Decision
Plan Alpine native packaging as a post-V1 deliverable while preparing V1 build outputs to be packaging-ready.

## Rationale
- Preserves V1 focus on core gallery functionality.
- Reduces delivery risk by separating product completeness from distribution hardening.
- Aligns with user requirement: complete Alpine support is required but not mandatory in V1.

## Consequences
- Post-V1 milestone must include package format, service integration, and install docs.
- V1 should still emit stable binaries and directory conventions compatible with later packaging.

## Rejected Alternatives
- Forcing complete packaging in V1: higher schedule risk for core feature delivery.
- Container-only delivery: does not satisfy native package target requirement.

## Follow-ups
- Decide package format and init integration details.
- Define filesystem layout, service user, and runtime directories.
- Add upgrade and rollback guidance.
