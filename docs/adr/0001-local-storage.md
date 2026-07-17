# ADR 0001: Local cache and source ownership

- Status: Accepted
- Date: 2026-07-16

## Context

RLViz needs fast navigation and search over large traces without changing or copying the user's source artifacts unnecessarily.

## Decision

Source artifacts remain read-only and authoritative. RLViz stores derived indexes, normalized metadata, and cache provenance in a separate per-user cache directory. Cache entries are keyed by the resolved source path, source fingerprint, adapter identity, and protocol version.

The initial single-trajectory slice may parse a small canonical file in memory. Persistent indexing will use SQLite through a pure-Go driver and retain source byte offsets wherever the adapter can provide them.

## Consequences

- Removing the RLViz cache never removes or changes source data.
- A stale source fingerprint invalidates derived data.
- `rlviz cache status` and `rlviz cache clean` must make disk use visible and controllable before persistent indexing ships.
- Raw records are read from the registered source when practical instead of being treated as owned copies.
