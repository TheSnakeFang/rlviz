# Supported formats

This document is the public source-of-truth for what RLViz can open. Keep it
literal; examples and planned support are not built-in support.

## Built in

### Canonical NDJSON (`rlviz.dev/v1alpha1`)

RLViz directly decodes newline-delimited canonical records matching the schemas
under `schemas/v1alpha1/`. A valid stream emits parent records before children,
orders events strictly within each trajectory, and ends with one `complete`
record.

The smallest examples are under `fixtures/canonical/`.

The CLI and static browser viewer also include pure-Go mappings for these
documented JSON shapes:

- Harbor Agent Trajectory Interchange Format (ATIF) v1.5-v1.7
- Letta trajectory v1 normalized record arrays
- Inspect AI EvalLog JSON version 2 (not `.eval` archives)
- Prime Intellect Verifiers GenerateOutputs JSON

Those mappings compile into both the native binary and local Go WASM core.

### Letta trajectory v1 JSON

RLViz recognizes the public [trajectory v1 schema](https://github.com/letta-ai/trajectory/blob/main/schema/trajectory-v1.schema.json)
as a JSON array beginning with one `meta` record. User, reasoning, assistant,
tool-call, and linked tool-result records become source-backed timeline events.
Harness, model, working-directory, and branch metadata are retained when
present.

This opens normalized output from `@letta-ai/trajectory`; it does not directly
claim support for every native harness format that package can normalize. Save
the `records` array returned by `normalizeTranscript`, not the surrounding
result object. Fields omitted or truncated during upstream normalization cannot
be reconstructed by RLViz.

### Harbor ATIF JSON

RLViz directly recognizes the public [Harbor Agent Trajectory Interchange
Format](https://github.com/harbor-framework/harbor/blob/main/rfcs/0001-trajectory-format.md)
versions 1.5 through 1.7. It maps messages, reasoning, tool calls and correlated
observations, metrics, multimodal image references, and v1.7 embedded subagents.
External `trajectory_path` and continuation references are preserved but never
followed automatically.

The reader accepts the RFC's one-based step IDs and historical Harbor v1.5-v1.6
exports that begin at zero. Negative IDs remain invalid.

This support is deliberately limited to an ATIF trajectory document. Harbor
job directories, evaluator outputs, rewards stored beside the trajectory, and
organization-specific metadata require a local adapter. No private schema or
customer fixture is part of the built-in mapping.

The browser retains its 32 MiB per-source ceiling. The local CLI is the intended
path for longer trajectories and persistent indexes.

## Example adapter

### Simple JSONL

`examples/adapters/simple-jsonl` demonstrates a process adapter for a small
non-canonical JSONL format. It is reference code, not automatically discovered
or trusted built-in support.

### Inspect AI EvalLog JSON

`examples/adapters/inspect-ai` remains as readable reference code for the same
built-in documented JSON `EvalLog` shape,
including model, tool, score, and compaction events. It intentionally does not
claim support for the compressed `.eval` container.

### Prime Intellect Verifiers GenerateOutputs JSON

`examples/adapters/verifiers` remains as readable reference code for the same
built-in JSON-compatible `GenerateOutputs`
contract, including rollout steps, rewards, metrics, token masks, and explicit
generation truncation flags.

These executable examples are dependency-free and backed by synthetic,
contract-shaped fixtures. The Simple JSONL example is not built in; Inspect and
Verifiers are built in but keep examples to document the adapter contract.

## Project-local and user adapters

Private or organization-specific formats are supported through trusted process
adapters. Today an adapter is selected explicitly:

```bash
rlviz open SOURCE --adapter .rlviz/plugins/my-format
```

`rlviz formats` discovers bounded manifest metadata from project and user plugin
directories. This inventory does not execute probes, select an adapter, or grant
trust. Bounded automatic probing remains future work. Documentation and CLI
output must not imply that RLViz natively recognizes arbitrary JSON, databases,
directories, or vendor formats.

## Format-support policy

A format can become built in when it has:

- a stable or well-documented source contract
- redistributable synthetic fixtures
- deterministic mapping to canonical semantics with source provenance
- bounded probing and streaming behavior
- conformance, malformed-input, and large-input tests
- a maintainer path for upstream format changes

Formats with private schemas, rapid internal churn, or customer-specific fields
should remain adapters. Useful adapters may be published separately without
expanding the core binary.

`rlviz inspect [--json] [--adapter PATH] SOURCE` performs bounded, read-only
format detection without starting the daemon or browser. An explicit adapter
must already be trusted; inspection invokes its `probe` operation but never
`stream`.

`rlviz formats [--json] [--project DIR] [--plugin-root DIR]...` generates a
schema-versioned inventory from the built-in decoder, current trust store, and
bounded manifest discovery. It reports unavailable, digest-changed, untrusted,
and invalid entries without executing them. Discovery rank reflects only the
documented root and path order; it is not a compatibility score.
