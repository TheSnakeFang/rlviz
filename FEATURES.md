# RLViz features and workflows

This is the product-facing feature registry. Use it to scope work, write tests,
and keep the site and docs aligned with the viewer. A feature is `current` only
when its primary workflow is implemented and covered at the appropriate layer.

Status: `current` ships in the working product, `next` is committed near-term
work, and `later` is an intended capability without a committed release.

## Supported workflows

| Workflow | User outcome | Status |
| --- | --- | --- |
| Open a local trace | Run `rlviz open PATH`; inspect it in a loopback-only viewer without mutating or uploading it | current |
| Open in the browser | Drop a modest trace into the browser viewer; parsing and indexing stay in the tab | current |
| Browse a collection | Filter rollouts, change visual fidelity, switch between a flat rollout list and trial groups, and open or add a rollout | current |
| Inspect a rollout | Move through events and landmarks, change depth and fidelity, adjust the timeline viewport, and inspect normalized and raw source data | current |
| Build a workspace | Arrange rollout modules in rows or columns, move and resize modules, keep context lanes, and serialize the layout into a deep link | current |
| Keep one rollout in detail | Open a rollout-pinned detail module, navigate it directly, and inspect other lanes without replacing it | current |
| Triage a rollout group | Inspect outcome and metric distributions, sort or filter trajectories, and jump to representative or failing rollouts | current |
| Compare behavior | Align two trajectories around behavioral anchors and inspect their first meaningful divergence and structured differences | current |
| Extend an unsupported format | Scaffold, trust, validate, and run a local adapter without changing the viewer | current |
| Configure the viewer | Complete first-run setup and edit layout, theme, fidelity, and keybindings in one documented config | next |
| Inspect alignment in context | Overlay aligned reference events and divergence evidence directly on rollout lanes | next |
| Hand work to an agent | Generate a local workbook that gives a coding agent bounded trace context and explicit tasks | next |
| Use one public surface | Read the short product explanation, install, open a trace, and continue into the viewer on `rlviz.dev` | next |

## Feature areas

### Local data and formats

- `current` Canonical run, case, rollout-group, trajectory, event, signal, and artifact model.
- `current` Built-in format detection plus process-isolated adapters and analyzers.
- `current` Source provenance down to line, byte range, or original raw record.
- `current` Read-only source handling, removable caches, loopback binding, and no outbound requests during normal viewing.
- `current` Progressive indexing and updates for growing local files.
- `next` Clear first-run format diagnostics and an in-product adapter handoff.

### Collections and triage

- `current` Keyboard-first filtering and rollout selection with visible-selection follow.
- `current` Wheel scrolling inside the collection module.
- `current` Flat rollout view and trial-grouped view.
- `current` Three truthful collection fidelity levels: hairline, glyphs, and detail.
- `current` Outcome, reward, pass, error, token, latency, retry, and termination signals when present.
- `later` Saved collection presets for recurring evaluation suites.

### Rollout inspection

- `current` Four semantic depths: overview, episodes, events, and source.
- `current` Independent zoom and timeline viewport controls.
- `current` Overview fidelity from hairline to every visible step, including tool-call names.
- `current` Event, error, reward/grader, context, finding, artifact, and marker navigation.
- `current` Selected-event detail with payload, provenance, signals, and artifacts.
- `current` Pinned detail modules tied to one rollout.
- `later` Persisted analyst annotations and exportable evidence ranges.

### Workspace and interaction

- `current` Docked collection, rollout lanes, global detail, and rollout-pinned detail modules.
- `current` Rows-first additional rollouts, optional columns, pointer docking, and keyboard move/resize modes.
- `current` `Alt` + arrow spatial navigation and `Tab` cycling.
- `current` One always-visible keybar generated from the active module's command bindings.
- `current` Customizable keymap storage and active-shortcut help.
- `current` Serialized layouts, history, and shareable local deep links.
- `next` Onboarding and a complete keybinding-remap UI backed by the shared config.

### Comparison and derived analysis

- `current` Reference lanes and two-trajectory comparison.
- `current` Deterministic alignment on tool calls, environment actions, state, rewards, errors, and termination.
- `current` First meaningful divergence and later-event realignment.
- `current` Compact behavioral-path aggregation labeled as derived, never source-native branching.
- `next` Alignment overlays inside the main workspace.
- `later` Domain-specific analyzer overlays that remain distinguishable from source facts.

### Presentation and distribution

- `current` CLI daemon viewer and zero-upload WebAssembly browser viewer sharing one instrument UI.
- `current` Light/dark themes, presentation palettes, semantic glyphs, and accessible text equivalents.
- `current` Favicon, social preview, and plain metadata that describe the product without marketing claims.
- `next` One-site transition at `rlviz.dev` and an external-user docs rewrite.
- `next` Agent-readable workbooks and setup instructions.

## Product boundaries

RLViz is a local inspection and comparison tool. Hosted accounts,
collaboration, production alerting, training or evaluation execution, prompt
management, automatic LLM summaries, arbitrary viewer JavaScript, and tool
re-execution are not product goals unless this document is deliberately
changed first.

## How work maps to this file

Every product task should name one supported workflow and one feature area.
UI changes need an observable browser-flow assertion; data and protocol changes
need source-truth tests; privacy changes need both. Docs and site copy should
describe only `current` behavior as available and label `next` or `later`
behavior explicitly.
