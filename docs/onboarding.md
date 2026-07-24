# Quickstart

RLViz can open a trace in the browser with no installation, or run locally as a
native CLI for larger and private datasets.

## Browser

Open [rlviz.dev](https://rlviz.dev). The viewer starts with a bundled synthetic
checkout cohort and an in-workspace Guide module. Open or drop a local trace
when you are ready; the file is parsed in that tab and is not uploaded.

The browser accepts files up to 32 MiB in these formats:

- canonical RLViz NDJSON
- Letta trajectory v1 JSON record arrays
- Inspect AI EvalLog JSON
- Verifiers GenerateOutputs JSON
- Harbor ATIF JSON v1.5-v1.7

Settings can reload any bundled example, open your own file, change theme, or
load a reviewed browser adapter. The Guide opens to installation instructions
in the hosted viewer.

## CLI

Install the native binary with Homebrew:

```bash
brew install TheSnakeFang/tap/rlviz
```

Or use npm or the checksum-verifying shell installer:

```bash
npm install --global rlviz
curl -fsSL https://rlviz.dev/install.sh | sh
```

Check the installation and local runtime:

```bash
rlviz version
rlviz doctor
```

`doctor` reports paths, daemon state, browser availability, Python
availability, trusted adapters, and actionable problems. It does not open a
trace or execute a plugin.

Run `rlviz` with no arguments to restore the last usable source and workspace.
If there is no history, RLViz opens its bundled synthetic gallery. The same
fallback applies to `rlviz open`.

## Open a trace

Probe an unfamiliar source before starting the viewer:

```bash
rlviz inspect ./path/to/rollout.ndjson
```

Then open it:

```bash
rlviz open ./path/to/rollout.ndjson
```

RLViz starts or reuses a loopback-only daemon, builds a removable local SQLite
index, opens the browser, and returns immediately. It watches active files and
adds appended events without changing the source.

Every operational command has structured output for scripts and coding agents:

```bash
rlviz inspect --json ./path/to/rollout.ndjson
rlviz status --json
rlviz guide --json
rlviz trajectories ./path/to/rollout.ndjson --failed --group-by trial --json
rlviz workspace open ./path/to/rollout.ndjson --trajectory ID --json
```

## Learn with synthetic data

```bash
rlviz demo
```

The demo contains a 300-event coding trace, a 120-event research trace, and a
16-rollout checkout cohort. All three are deterministic and synthetic.

## Private formats

List built-in and discovered formats:

```bash
rlviz formats
```

If `inspect` reports an unsupported format, scaffold a project-local adapter:

```bash
rlviz plugin init --type adapter --from ./path/to/private.trace .rlviz/plugins/private-format
```

The scaffold contains executable code. Review it before binding trust to its
path and digest, then test and validate it:

```bash
rlviz plugin trust .rlviz/plugins/private-format
python3 .rlviz/plugins/private-format/test_adapter.py
rlviz plugin validate .rlviz/plugins/private-format ./path/to/private.trace
rlviz open ./path/to/private.trace --adapter .rlviz/plugins/private-format
```

Read [adapter authoring](adapter-authoring.html) for the protocol, deterministic
IDs, provenance, and browser adapters.

## Optional setup

`rlviz init` configures the browser viewer and can preview version-matched
instructions for Codex, Claude Code, or Cursor. Every file is
shown before an explicit write confirmation. Existing files are never replaced.

The lower-level commands are useful in automated environments:

```bash
rlviz setup agent codex --print
rlviz setup agent codex --dry-run --destination .agents/rlviz.md
rlviz setup agent codex --write --destination .agents/rlviz.md
```

Writes are create-only and must stay inside the current project.

## Local state

Inspect or remove the local index after stopping the daemon:

```bash
rlviz cache status
rlviz stop
rlviz cache clean
```

Cleanup removes the SQLite index and its journal files. It never removes or
rewrites source traces.
