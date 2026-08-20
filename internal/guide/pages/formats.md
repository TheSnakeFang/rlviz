# Formats and adapters

RLViz normalizes every source into versioned canonical run, case, group, trajectory, event, signal, artifact, and completion records. The GUI stays format-independent.

Built in: RLViz canonical NDJSON, Letta trajectory v1 JSON, Harbor ATIF v1.5-v1.7, complete local Harbor job directories, Inspect AI EvalLog JSON v2, and Verifiers GenerateOutputs JSON. Letta support opens the normalized `records` array, not native harness logs. ATIF support covers the public trajectory document, including embedded subagents. The native Harbor job source combines trial results, ATIF, CTRF, rewards, token and cost contexts, exceptions, and artifacts without following symlinks or external references. Organization-specific evaluator files remain adapter territory.

```sh
rlviz inspect ./jobs/example-job --json
rlviz open ./jobs/example-job
```

Harbor job directories are snapshotted when opened and are not live-watched in this release. Reopen a changed job to refresh its local index. The static browser accepts individual supported files, not complete job folders.

## Before writing an adapter

```sh
rlviz formats --json
rlviz inspect ./trace --json
```

If inspection reports `unsupported_format`, use the suggested scaffold command. Do not rename fields or rewrite the trace to imitate another format.

## Browser adapters

A coding agent can build an import-free WebAssembly adapter for a private browser-only format. Review the module and its SHA-256 digest, then upload it in Settings. It runs in the browser sandbox for the current session and is not persisted.

## Local plugins

The CLI can scaffold a project-local process adapter. Review every executable file and synthetic fixture before trusting it. Validation executes the adapter, and any edit changes its digest and requires review again.

```sh
rlviz plugin init --json --type adapter --lang python --from ./trace .rlviz/plugins/local
rlviz plugin trust --json .rlviz/plugins/local
rlviz plugin validate --json .rlviz/plugins/local ./trace
rlviz open --adapter .rlviz/plugins/local ./trace
```

Adapters map source data. Analyzers add deterministic findings. Declarative presentation changes labels, columns, inspector sections, theme tokens, and keybindings. Plugins never inject arbitrary JavaScript or CSS into the viewer.
