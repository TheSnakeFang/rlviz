# Workspace specification (v3)

Supersedes v2 and the mode model in `interaction-spec.md` §1. Everything else
in the interaction spec (selection, depth/zoom semantics, compare alignment,
tiers, command-ID rules, color budget) remains binding. RLViz is a **generic
trajectory viewer** — for one trajectory or many — not a triage or
observability product. Cohort-analysis capabilities arrive later as explicit,
opt-in modes.

## 0. Non-negotiable rendering and copy rules

1. **Truth-first rendering.** Every visual mark corresponds to a real event at
   its true axis position. Density is handled by *aggregation* (binning),
   never by geometric scaling; marks and text have fixed pixel sizes.
   Errors, context events, and evidence marks survive aggregation as
   guaranteed-visible landmarks. Anything decorative is deleted.
2. **No decor text.** Every visible string must earn its place: labels only
   where the user needs the word to act. No zone eyebrows (RAIL/CONSOLE),
   no taglines, no internal vocabulary ("surface", "focus band", "context
   band") in the UI. Depth positions use plain words — overview, episodes,
   events, raw — shown only as the active lane's breadcrumb.
3. **A legend exists.** The `?` overlay includes a marks key explaining every
   glyph and color in one screen.

## 1. Modules and docking

The screen is a set of **modules** in a docked layout, managed by a
constrained VSCode-style docking engine (implementation: `dockview-react`,
MIT, with floating panels and popout windows disabled):

- Modules: **collection** (the trajectory list), **lane** (one trajectory,
  0..N instances), **detail** (selected event content — the former console,
  now a peer module, docked right by default when lanes flow as rows),
  and future modules (compare readout, artifacts).
- Users move modules between dock positions by drag or keyboard; seams
  resize; **empty dock areas collapse to nothing** (no labeled voids).
- Deep links serialize logical modules, selection, depth, and viewport state.
  Exact Dockview geometry stays in bounded, versioned local storage and is
  restored only when the local panel topology matches the link. The flow
  harness enumerates and tests arrangements exactly as before.
- Keyboard: `Tab`/`Shift+Tab` cycle modules; **arrow-key spatial navigation**
  (`Alt+←↑↓→`) moves focus to the neighboring module. `Ctrl+m` enters a
  move-module mode where the same arrows relocate the active module; `Ctrl+w`
  enters seam-resize mode. The entry chord toggles its mode off and `Esc`
  exits either mode. While a mode is active, the keybar shows only its live
  arrow, toggle, and cancel controls.

## 2. The collection module

Collection and trajectory titles and descriptions may be edited as local
presentation metadata. These labels stay in browser storage and never modify
the source trace or become part of a shared workspace URL.

- One list, one representation control: the **fidelity ladder** `[` `]` with
  exactly three levels, each with a stated purpose:
  - **hairline** — the cohort's shape at a glance: one thin strip per
    trajectory, length ∝ events, landmarks (errors, context) at their true
    positions. Answers *"what does this set look like?"*
  - **glyphs** — one glyph per real event (kind-mapped, truncation shown
    honestly with a count), errors in place. Answers *"what happened in
    this one?"*
  - **detail** — glyph strip plus columns (events, reward, source) and
    metadata. Answers *"which one is this exactly?"*
- Default ordering is **source order**. No attention ordering, no
  "unresolved" counts, no verdict tags in the default product.
- Filter narrows by substring. The explicit **rollouts** view is flat source
  order; **trials** groups the same rows by case/group identity without
  changing keyboard order or selection. Later sidecars may add named buckets.
  The collection is its own scroll container and `j/k` keeps the selected row
  visible.
- `Enter` opens in the active lane (focus moves to the lane); **`a` adds a
  lane and keeps focus in the collection** for rapid multi-add; `x` closes
  the active lane.

## 3. Lanes

- A lane renders one trajectory at a **depth** (overview → episodes →
  events → raw) with an **axis zoom** (`+ - 0`, anchored, ascent restores
  the pre-descend axis). At overview depth, `[` / `]` use the same
  hairline → glyphs → detail fidelity ladder as the collection. Detail
  fidelity names every visible real step, including its tool-call name when
  present. Fidelity does not replace depth and is inactive below overview.
- The strip renders in pixel space: fixed-size marks, positions from
  measured width; when density passes the legibility threshold the nominal
  marks aggregate into density bars (waveform-style) while landmark events
  stay discrete.
- Lane count is unbounded; lanes stack as rows by default so adding rollouts
  never squeezes each one into another narrow column. Additional lanes may
  render as thin rows in a collapsible dock area. `Shift+Enter` swaps a thin
  lane with a full one. `n`/`p` sweep the active lane through the collection's
  current filtered order.
- Every lane has a full-rollout timeline at its bottom. It shows the current
  axis window. Clicking recenters it, dragging pans it, and dragging either
  edge resizes that side, like a video editor's viewport control.
- `d` opens a detail module pinned to the active rollout. Its `j`/`k` and
  landmark keys operate on that rollout even after another module becomes
  active; closing the pinned detail does not close the rollout lane.

## 4. Keys: one bar, always accurate

A single keybar at the bottom of the screen renders the **active module's
actual bindings** from the command registry (top ~8 by relevance, `?` for
all). Per-module footers are removed. Every binding shown is live; every
live binding is discoverable through `?`. Clicking a keybar chip executes
its command. Dock content cannot shrink, cover, or scroll this bar away.

## 5. Settings, onboarding, and agents

- All preferences are **data** in one config file (Tier C): keymap remaps,
  palette, default fidelity, layout/docking arrangement, per-trace-type
  presets (glyph bindings, landmark definitions, episode anchors).
- **First-run onboarding**: an in-app setup flow (and `rlviz init` in the
  CLI) that walks layout, theme, and keymap choices and writes the config.
- The config file's location and schema are documented so **the user's
  coding agent can adjust it** — trace-type-specific setup (private event
  kinds, custom glyphs, episode anchor rules) is expected to be done by
  agents; RLViz ships correct deterministic defaults for everything
  universal.
- Hosted viewer stays display-only: no agent hooks, no MCP, no network for
  trace data. Local CLI is the agent surface.

## 6. One site

`rlviz.dev` is the only URL: a short product landing (what it is, install,
open-a-trace drop zone) where dropping a file transitions the page into the
viewer in place. `app.rlviz.dev` redirects. Docs are written for external
users (what/why, quickstart, formats, adapters, FAQ); contributor and
architecture docs stay in the repo.

## 7. Flow QA

Unchanged in structure. Arrangements remain enumerable via layout
serialization; flows must cover module moves, arrow navigation, empty-dock
collapse, fidelity ladder (three levels, each with a distinguishing
truthful observable), and the density-binning threshold (a marks-mode strip
and a binned strip assert different structures on the same data at
different widths).

## 8. Build order (agreed Jul 22)

1. Truth-first rendering nucleus: strip layout math (binning, pixel space)
   + honest collection strips + legend; remove per-lane fidelity; remove
   triage defaults and decor text.
2. Keybar + discoverability + interaction fixes (add-stays, arrows,
   empty-collapse, close affordance).
3. Docking migration to dockview (constrained), detail module to the side,
   layout serialization + onboarding flow.
4. One-site merge + user-facing docs rewrite.
5. Then phases 3–4 from v2 (alignment overlay, agent workbooks) on top.
