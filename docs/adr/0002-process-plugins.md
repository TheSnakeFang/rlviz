# ADR 0002: Versioned process plugins

- Status: Accepted
- Date: 2026-07-16

## Context

Private rollout formats vary widely. Coding agents should be able to create adapters in the language and repository where that format already lives, without recompiling RLViz.

## Decision

Adapters and future analyzers are subprocesses that exchange versioned JSON requests and NDJSON records over standard streams. The public protocol begins at `rlviz.dev/v1alpha1`.

Project-local plugins are discoverable but never implicitly trusted. Execution trust is recorded against the plugin's resolved path and content digest. Changing executable content requires renewed trust.

Arbitrary third-party JavaScript is not loaded into the main viewer. Initial presentation extensions are expressed through canonical records and declarative metadata.

## Consequences

- Plugins are portable across RLViz builds and can be authored in Python, Go, TypeScript, or another local runtime.
- Standard output is reserved for protocol records; diagnostics use standard error.
- Process startup and serialization cost are accepted in exchange for portability and isolation.
- Compatibility is tied to the declared protocol version rather than RLViz's internal Go packages.
