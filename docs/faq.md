# FAQ

## Does RLViz upload traces?

No. The browser viewer reads files through the browser `File` API and keeps its
index in memory. The CLI binds to loopback, reads local sources, and makes no
outbound requests during normal viewing.

Package installation and opening external documentation are separate network
actions. A local process adapter is executable code and has the permissions of
the user running it, which is why trust requires explicit path-and-digest
confirmation.

## Does it change the source trace?

No. Source files are read-only. The CLI writes a removable SQLite index in its
own state directory. Editable titles, descriptions, shortcuts, theme, and dock
geometry are presentation state stored on the local device.

## Why use this instead of a notebook?

A notebook remains useful for custom analysis. RLViz covers the repeated
inspection work: event navigation, raw provenance, rollout collections,
adjustable detail, multi-rollout layouts, and deterministic comparison. It can
open a new trace without writing a new parsing and display path each time.

## Does a run need to be instrumented with RLViz first?

No. RLViz reads artifacts from runs that already happened. Built-in decoders or
local adapters map the stored format into canonical events.

## What can the browser open?

Canonical RLViz NDJSON, Letta trajectory v1 JSON, Harbor ATIF v1.5-v1.7, Inspect AI EvalLog JSON, and Verifiers GenerateOutputs
JSON up to 32 MiB. The browser holds source bytes, canonical records, and the
browse index in tab memory, so larger files and cohorts belong in the CLI.

## How do private formats work?

The CLI runs an explicitly trusted local process adapter. The browser can run an
import-free WebAssembly adapter for one confirmed session. Both emit the same
canonical records; neither can add arbitrary UI code.

## Can I trace a normalized event back to the source?

Yes. Canonical events retain source provenance such as a line, byte range, or
raw record. Derived comparisons and findings point back to canonical events.

## Does RLViz execute recorded tool calls?

No. Recorded messages, tool calls, outputs, artifacts, and environment actions
are data to inspect. RLViz never replays them.

## Is this an observability or evaluation platform?

No. RLViz does not run evaluations, train models, manage prompts, host traces,
send alerts, or provide accounts and collaboration. It is a local inspection
and comparison tool.

## Where is the product roadmap?

Shipped, next, and later work is tracked in the repository's
[feature registry](https://github.com/TheSnakeFang/rlviz/blob/main/FEATURES.md).
