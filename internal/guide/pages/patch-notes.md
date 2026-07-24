# Patch notes

## 0.3.5

- Added a two-pane compact workspace for 720–1199px windows and a single-module touch workspace below 720px.
- Added a one-time inline compact-view notice without blocking the demo.
- Preserved open modules, event selection, shared links, and desktop dock geometry while resizing.

## 0.3.4

- Added built-in Harbor ATIF v1.5-v1.7, Inspect AI EvalLog v2, and Verifiers GenerateOutputs adapters across the CLI and browser viewer.
- Preserved ATIF tool calls, results, metrics, attachments, and embedded subagent trajectories in RLViz's canonical event model.
- Streamed trusted adapter output directly into transactional SQLite and paged local events in 1,000-event windows for long-running rollouts.
- Reduced duplicate indexed payload data, cutting requests and memory use on large traces.

## 0.3.2

- `Enter` now opens the selected rollout section in place and clears an unpinned Detail module out of the way.
- `d` opens or closes the shared Detail module. `Shift+D` pins it to one rollout; `Shift+C` compacts or expands it.
- Sandboxed sites can embed `rlviz.dev`; trace handling still stays inside the viewer frame.

## 0.3.1

- Added rollout spotlight, spatial arrow-key module navigation, and `Shift+R` workspace reset.
- Replaced ambiguous collection glyph strips with compact, signals, and summary views.
- Added agent-oriented CLI workspace commands for opening, grouping, and updating the GUI.

## 0.3.0

- Introduced the dockable multi-rollout workspace, timeline navigator, pinned detail modules, Guide, and Settings.
