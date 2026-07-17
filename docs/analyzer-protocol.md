# Analyzer protocol

RLViz analyzers inspect canonical events and signals and return findings and
derived signals. They do not call a language model. Analysis is a pure operation:
the same analyzer digest and normalized input must produce byte-equivalent JSON.

The analyzer boundary is implemented in `internal/analyzers`. External analyzers
run locally through the same explicit-trust and immutable-snapshot boundary as
adapters.

External analyzer CLI execution is available through `rlviz plugin validate`.
The public machine-readable contracts are
`schemas/v1alpha1/analyzer-input.schema.json` and
`schemas/v1alpha1/analyzer-output.schema.json`; conforming examples live under
`fixtures/protocol/`.

## Manifest

An external analyzer is a normal trusted process plugin. It uses the same content
digest, explicit trust, immutable execution snapshot, timeout, output limit, and
process-tree termination rules as an adapter.

```yaml
api_version: rlviz.dev/v1alpha1
kind: Analyzer
name: repeated-actions
version: 0.1.0
command: [python3, analyzer.py]
capabilities: [analyzer.analyze]
description: Finds repeated action patterns
```

`Analyzer` plugins must declare exactly `analyzer.analyze`. Adapter capabilities
cannot be mixed into an analyzer manifest. Trust is attached to the resolved
plugin path and full content digest, so changing the manifest, implementation,
local dependencies, or executable invalidates approval.

## Input

The host writes one private JSON request file and invokes the analyzer using the
same convention as adapters:

```bash
analyzer analyze --request request.json
```

The request contains one trajectory's canonical events and signals:

```json
{
  "api_version": "rlviz.dev/analyzer/v1alpha1",
  "operation": "analyze",
  "trajectory_id": "trajectory-1",
  "events": [
    {
      "record_type": "event",
      "id": "event-1",
      "trajectory_id": "trajectory-1",
      "sequence": 0,
      "kind": "tool",
      "input": {"name": "shell", "arguments": {"command": "go test ./..."}}
    }
  ],
  "signals": []
}
```

Events are normalized by `(sequence, id)` and signals by `id` before hashing.
All records must belong to `trajectory_id`; event sequences must be unique and
strictly increasing after normalization; signal event references must resolve.
The initial limits are 100,000 events, 100,000 input signals, and 64 MiB encoded
input. A SHA-256 digest of the normalized JSON is the input identity. The
request file is ephemeral, mode `0600` on POSIX systems, and read-only input to
the plugin; the host removes it after execution. The plugin receives no stdin.

## Output

The analyzer writes one JSON value to stdout:

```json
{
  "api_version": "rlviz.dev/analyzer/v1alpha1",
  "provenance": {
    "name": "repeated-actions",
    "version": "0.1.0",
    "digest": "sha256:<plugin-content-digest>",
    "input_digest": "sha256:<normalized-input-digest>"
  },
  "findings": [
    {
      "id": "finding-1",
      "trajectory_id": "trajectory-1",
      "event_ids": ["event-1"],
      "kind": "retry",
      "severity": "warning",
      "title": "Repeated identical action",
      "summary": "The same action was attempted three times.",
      "fingerprint": "sha256:<behavior-digest>"
    }
  ],
  "signals": [
    {
      "record_type": "signal",
      "id": "signal-finding-1",
      "trajectory_id": "trajectory-1",
      "event_id": "event-1",
      "name": "analyzer.loop_retry.detected",
      "value": true
    }
  ]
}
```

Provenance must match the selected plugin's manifest and trusted content digest;
`input_digest` must match the request. Finding and signal IDs must be unique,
event references must resolve, severity is `info`, `warning`, or `error`, and
title/summary fields are limited to 16 KiB each. Output is capped at 1,000
findings, 1,000 signals, and 16 MiB encoded JSON. Analyzer output is supplemental:
it never changes the source canonical stream.

The host supplies `RLVIZ_ANALYZER_NAME`,
`RLVIZ_ANALYZER_VERSION`, `RLVIZ_ANALYZER_DIGEST`, and
`RLVIZ_ANALYZER_INPUT_DIGEST` so the process can populate provenance
without hard-coding its own content digest. Stdout must contain exactly one JSON
object and no diagnostics. Stderr is bounded to 1 MiB. Cancellation or the
default 10-second timeout terminates the analyzer process tree.

## External analyzer authoring

Create a dependency-free Python analyzer scaffold and inspect it before trust:

```sh
rlviz plugin init --type analyzer --lang python ./plugins/my-analyzer
rlviz plugin trust ./plugins/my-analyzer
rlviz plugin validate ./plugins/my-analyzer ./plugins/my-analyzer/sample-input.json
```

Validation executes the trusted snapshot twice and requires byte-identical
output. The validation input is a strict analyzer v1alpha1 request JSON object;
unknown fields, trailing values, invalid references, and oversized input are
rejected before plugin execution. Any implementation change invalidates trust.

## Built-in loop/retry analyzer

`builtin.loop-retry` provides a fast baseline without an LLM. It fingerprints
`tool` and `environment_action` events from their kind, input, and data. It
deliberately excludes IDs, timestamps, output, source offsets, and metadata so a
failed action retried after a different observation still compares equal.

Three identical behavioral actions produce a `retry` finding. Three consecutive
copies of a two-to-four-action pattern produce a `loop` finding. Non-action
events do not interrupt the behavioral sequence. Findings contain all involved
event IDs and a stable behavior fingerprint; a derived boolean signal points at
the final event. Detector configuration, output, IDs, and provenance are fully
deterministic.

This heuristic is deliberately conservative and exact-match based. Semantic
similarity, learned classifiers, cross-trajectory analysis, and viewer-integrated
external analyzer selection are later layers on the same protocol; external
analyzer process execution and deterministic CLI validation are available now.

## Local indexed API

The built-in analyzer is available from the daemon's authenticated indexed API:

```text
GET /api/v1/indexed/analysis?trajectory=<source-id>&trajectory_id=<trajectory-id>&analyzer=loop-retry
Authorization: Bearer <daemon-token>
```

`trajectory_id` defaults to the source's first trajectory. `analyzer` defaults
to `loop-retry`; `builtin.loop-retry` is also accepted. The response contains
`analysis`, `cached`, and `analyzed_at`. Results are stored in the local SQLite
index under source ID, trajectory ID, analyzer name/version/digest, and normalized
input digest. Replacing a source deletes its results transactionally through the
same foreign-key cascade as other derived index data. A cache hit is decoded and
fully revalidated before it is returned.
