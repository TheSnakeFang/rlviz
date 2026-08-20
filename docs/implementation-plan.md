# Implementation plan

The implementation is organized as vertical milestones. Each milestone should leave a usable, testable product rather than a collection of disconnected subsystems.

## Current delivery

Milestones 0–7 are implemented in the repository: versioned contracts and fixtures, the single-trajectory viewer, authenticated daemon lifecycle, progressive SQLite indexing, paginated and virtualized reads, growing-file updates, trusted external adapters and analyzers, rollout-group summaries, compact behavioral paths, deterministic long-trace comparison, cached loop/retry findings, release automation, and verified installers. `v0.1.0` is published with native macOS/Linux archives, checksums, attestations, a curl installer, the `TheSnakeFang/tap/rlviz` Homebrew formula, and the `rlviz` npm package. npm trusted publishing is configured for future tags.

## Milestone 0: contracts and fixtures

### Deliverables

- Canonical entity and event schemas
- Plugin manifest schema
- Adapter request and NDJSON response schemas
- Small linear trajectory fixture
- Rollout-group fixture
- Branched trajectory fixture
- Malformed and adversarial fixtures
- Architecture decision records for storage, plugins, and daemon lifecycle

### Exit criteria

- Schemas validate every fixture.
- Stable IDs and relationship constraints are specified.
- A contributor can understand the format without reading Go code.

## Milestone 1: single-trajectory vertical slice

### Deliverables

- `rlviz open`
- Foreground local server
- Built-in canonical JSONL adapter
- Embedded React UI
- Three-pane trajectory layout
- Event selection and raw payload inspection
- Basic keyboard navigation
- Text, JSON, image, and log artifact renderers

### Exit criteria

- One release binary opens the canonical fixture.
- Every rendered event links to its raw source record.
- The source remains unchanged.
- Browser tests cover the primary navigation flow.

## Milestone 2: daemon, streaming, and large files

### Deliverables

- Background daemon lifecycle
- Loopback authentication token
- SQLite metadata and search index
- Incremental JSONL parsing
- Source byte offsets
- Paginated HTTP API
- Virtualized event list
- File watching and append-only updates
- Cache status and cleanup commands

### Exit criteria

- `rlviz open` returns without holding the invoking shell.
- A 10,000-event trajectory scrolls smoothly.
- Large files reach first render before full indexing completes.
- Appended records appear without reopening the viewer.

## Milestone 3: external adapters and agent workflow

### Deliverables

- Plugin discovery and manifest parsing
- Adapter `probe` and `stream` process host
- Trust-by-path-and-digest flow
- `plugin init`, `list`, `validate`, and `doctor`
- Python adapter template
- Golden fixture test harness
- Structured CLI diagnostics
- Claude Code, Codex, and Cursor integration instructions

### Exit criteria

- A coding agent can scaffold an adapter from an unsupported sample.
- The validator identifies the exact invalid record and field.
- A trusted Python adapter opens a source without rebuilding RLViz.
- A changed adapter is not executed until trusted again.

## Milestone 4: rollout groups

### Deliverables

- Run, case, and group navigation
- Trajectory summary table
- Reward, pass, length, token, latency, error, and termination summaries
- Sorting and filtering
- Best/worst shortcuts
- Behavioral path fingerprints

### Exit criteria

- A researcher can identify representative success and failure trajectories without opening each one.
- Aggregates are computed incrementally from indexed fields.
- Group membership remains source-native and distinguishable from user selections.

## Milestone 5: pair comparison and divergence

### Deliverables

- Comparison-set model
- Deterministic event fingerprints
- Sequence alignment engine
- Aligned two-lane viewer
- Common-prefix compression
- First meaningful divergence marker
- State, reward, artifact, and termination differences
- Adapter-provided alignment-key support

### Exit criteria

- Equivalent actions align despite irrelevant text differences.
- Insertions, deletions, retries, and later realignment remain understandable.
- Alignment output has deterministic golden tests.
- Users can deep-link to a divergence.

## Milestone 6: compact paths and analyzers

### Deliverables

- Aggregated behavioral-prefix tree
- Explicit distinction between derived paths and source-native branches
- Analyzer plugin protocol
- Loop and retry analyzer
- Domain-specific signal output
- Cached analysis results with provenance

### Exit criteria

- A rollout group can be summarized without a spaghetti graph.
- Analyzer results identify plugin name, version, and input digest.
- Removing analyzer output never changes source data.

## Milestone 7: open-source release quality

### Deliverables

- macOS arm64/x64 and Linux arm64/x64 releases
- Checksums and GitHub artifact attestations
- Homebrew tap
- Reproducible release workflow
- Security policy and threat-model review
- Performance benchmarks
- Adapter-authoring tutorial
- Public example dataset

### Exit criteria

- A clean machine can install and open the example in under one minute.
- Release artifacts require no language runtime.
- Normal viewing makes no outbound requests.
- Documentation covers unsupported formats and safe plugin review.

## Milestone 8: product and design foundations

Status: implemented for the trajectory workspace; screenshot automation and
additional representative real-format fixtures remain ongoing quality work.

### Deliverables

- Current system architecture and change map
- Researcher-centered UI information architecture
- Design tokens, typography, density, and core component primitives
- Central command registry and rebindable local keymap
- Representative rich, context-compaction, verifier, group, and long-run fixtures
- Initial real-browser, screenshot, and accessibility quality gates

### Exit criteria

- The UI has one dominant reading surface and consistent supporting panels.
- Shortcut help, rebindable settings, and displayed key hints come from one registry.
- Essential text remains readable in comfortable and compact density.
- Visible workflows have deterministic browser coverage and visual evidence.

## Milestone 9: expert onboarding

Status: rich demo, format inventory, bounded inspection, and version-matched
agent setup are implemented for Codex, Claude Code, and Cursor. Setup writes
are explicit and create-only. Source-aware adapter scaffolding now returns a
versioned review plan plus a bounded, value-free structural profile without
copying sample records. Bundled agent instructions consume that profile before
targeted source inspection, enforce implementation and explicit review before
trust, retain the source-aware `--from` handoff, and use JSON for every
machine-operated command. JSON-mode setup usage errors use the stable
diagnostic envelope. Generated adapter projects include a strict synthetic-case
manifest and path-safe runner that delegates to the trusted Go validator.

### Deliverables

- Rich bundled `rlviz demo` (implemented)
- `rlviz formats [--json]` (implemented)
- Read-only `rlviz inspect [--json] [--adapter PATH] SOURCE` (implemented)
- Safe agent-integration print command (implemented)
- Explicitly reviewed agent-integration write/setup workflow
- Explicit supported-format documentation
- Improved unsupported-format and adapter-scaffold guidance
- Reviewed synthetic-fixture harness for generated adapters (implemented)

### Exit criteria

- A clean install reaches a representative demo in under one minute.
- Users can distinguish built-in, example, discovered, trusted, and unsupported formats.
- A coding agent can move from probe to reviewed adapter without matching human error text.
- Agent setup never overwrites existing project instructions.

## Milestone 10: research-grade trajectory reader

Status: transcript, event timeline, outcome/evidence, selected-event-first
details, a sparse semantic landmark rail with raw-result fallback, a source-backed
context track, context-change jump, long-run virtualization, and a bounded
whole-trajectory activity overview are implemented. Broader real-format context
evidence remains ongoing quality work.

### Deliverables

- Transcript and event-timeline modes
- Turn and tool-span grouping with raw-event fallback
- First-class outcome, verifier, reward, and final-output surface
- Context-usage track and compaction/truncation landmarks
- Selected-event-first details and evidence panel
- Semantic landmark rail and whole-trajectory minimap

### Exit criteria

- A researcher can explain the outcome without hunting through raw events.
- Context gained, lost, compacted, or restored is visible when the source provides it.
- Every grouped or derived surface links to canonical and raw source records.
- A 10,000-event trajectory remains keyboard-navigable and responsive.

## Milestone 11: group, divergence, and safe customization

Status: deterministic behavioral alignment, divergence navigation, and pair
summary deltas for outcome, tokens, explicit context events, compactions, and
source-shaped verifier results are implemented. Reproducible cohort filters
cover outcome fields, core metrics, and scalar signals. Selected tool arguments
and results have a bounded field-level diff. Reward-rank, outlier, failure, and
infrastructure shortcuts are command-registry backed. Optional metric and scalar
signal column layouts are bounded, locally persisted, and fail safe. The first
declarative presentation contract is wired end-to-end through explicit CLI
flags, independent daemon validation, persistent normalized storage, and the
trajectory API. Inspector section ordering/visibility and portable core-command
keymap defaults now use that same bounded contract.
Cross-source rollout queries add bounded pagination, source-backed
run/checkpoint/model/environment and tool filters, reward/token/cost ranges,
and full-cohort aggregates. Source-reported cost and deterministic tool-call
counts are normalized group metrics and configurable columns.

### Deliverables

- Cohort distributions, multi-signal filters, outlier shortcuts, and saved columns
- Outcome, verifier, context, and compaction deltas in pair comparison
- Synchronized detail and structured tool argument/result differences
- Declarative field, signal, inspector, theme-token, and keymap customization
- Bounded adapter discovery and deterministic inventory ranking without implicit executable trust

### Exit criteria

- Researchers can choose representative cohorts before opening individual runs.
- Comparison explains meaningful behavioral and context divergence, not only raw JSON difference.
- Customization uses validated core primitives and cannot inject arbitrary viewer JavaScript or CSS.

## Near-term issue sequence

1. Define the next trajectory workspace from an accepted information hierarchy,
   interaction model, and design specification before beginning a visual rewrite.
2. Validate the Inspect AI and Verifiers mappings against additional upstream
   samples as their public contracts evolve.
3. Validate landmark density and labeling on longer real research traces.
4. Validate the clean-machine install-to-open path on Linux; macOS curl,
   Homebrew, and npm paths are verified.

## Milestone 12: browser navigation reliability

Status: completed in `7e39117`.

Delivered:

- Playwright with a deterministic 1440 x 900 Chromium project
- reproduction and repair of landmark-rail scroll snap-back
- stable command handling across React renders
- input and modal suppression for global commands
- one-time deep-link and selection reveal behavior
- browser coverage for repeated keyboard navigation, manual rail scrolling,
  input suppression, Help focus return, and deep-link restoration

## Milestone 13: mobile-first progressive trajectory reader

Status: mobile reader implemented; progressive desktop carry-over remains. The
sub-720px projection now opens an outcome-first Summary, a virtualized Story,
source-backed Evidence, and canonical Details behind bounded bottom navigation.
It preserves the reading surface and selected event across reload and copied
workspace links without changing the desktop workspace.

The mobile reader is the design nucleus for the next workspace. A narrow screen
must establish the task, result, verifier explanation, and chronological story
without removing cohort controls, comparison, source records, or research
settings. Screen size changes the hierarchy and how much context can be shown at
once, not which evidence or capabilities exist. Wider layouts add simultaneity,
density, and faster multi-trajectory workflows rather than an exclusive expert
mode or a separate product.

### Vertical slices

1. **Implemented.** Replace the default Guide-first mobile state with a concise trajectory
   summary: task, agent/model, result, verifier reason, work, and one primary
   action to read the run. Keep Guide and Settings reachable but secondary.
2. **Implemented.** Define one scrollable story surface for user/model turns, tool calls,
   observations, errors, grader evidence, and artifacts. Preserve exact event
   identity and raw source access behind progressive disclosure.
3. **Implemented.** Replace the horizontally scrolling desktop-module strip with bounded mobile
   navigation for Browse, Story, Evidence, and Details. Use bottom actions or
   sheets for the selected event without hiding content behind a permanent
   desktop keybar.
4. Carry the same hierarchy into comfortable desktop mode, then layer on
   collection rails, multiple lanes, docking, pair comparison, configurable
   density, and keyboard workflows.
5. **Partially implemented.** Fixed 390 x 844 browser coverage now proves first
   use, outcome comprehension, Story and Evidence navigation, Details, deep-link
   reload, and horizontal fit. Extend the same coverage to long payloads, images,
   errors, empty states, and reduced motion.

### Exit criteria

- A first-time mobile user can identify the task, outcome, and source-backed
  reason and reach the decisive event without opening Guide.
- Essential controls use touch-sized targets, essential labels are not clipped,
  and the document never scrolls horizontally at the 390px reference viewport.
- A copied deep link restores the same trajectory, selected event, and reading
  surface after reload.
- The simplified default never removes canonical events, provenance, verifier
  evidence, raw records, comparison, or desktop research depth.

## Milestone 14: collection analytics and analysis overlays

Status: the server-side rollout query foundation, source-reported cost totals,
tool-call counts, typed browser client, configurable cost/tool columns, and
device-local trajectory labels/notes are implemented. The remaining event/span
annotation, span, usage-breakdown, and external analysis-result work is planned
and must retain source provenance.

### Remaining vertical slices

1. Validate a canonical generation-usage vocabulary across at least two real
   formats, covering input, cache-read, cache-write, output, and reasoning
   tokens without conflating billing totals with context occupancy.
2. Validate tool call/result and nested span linkage across representative
   formats before exposing success/failure, per-tool latency, output bytes,
   turn length, or span-error distributions.
3. Extend the implemented removable device-local trajectory labels/notes to
   event/span tags and evidence ranges. Never mutate the source trace.
4. Add a versioned external analysis-result import containing producer and
   input provenance, subject references, findings, scalar signals, tags, and
   optional DAG-node relationships. RLViz visualizes these results; MTG or an
   analysis service executes model-backed workflows.
5. Add collection histograms and facet controls backed by the paginated query
   API, with URL-stable filters and incremental page loading.

### Exit criteria

- Every distribution is computed from explicit indexed facts and reports its
  filtered population independently of page size.
- Tool success, span status, usage categories, and cost are absent rather than
  inferred when their source does not report them.
- Human annotations, source facts, adapter derivations, and analyzer findings
  are visually and structurally distinguishable.
- Viewing remains local-first and does not execute agents, recorded tools,
  provider billing lookups, LLM scanners, or analysis DAGs.

## Product expansion sequence

These phases follow the mobile reader. They deliberately keep the native RLViz
viewer local-first while allowing an explicit hosted companion to make selected
public trajectories and benchmarks shareable.

### Phase A: portable sharing and Harbor job interoperability

Status: in progress. RLViz opens Harbor ATIF v1.5-v1.7 trajectory JSON, the
native CLI opens complete local Harbor job directories, and both native and
static browser readers open reviewed digest-bound `.rlviz` bundles, including
tool/result correlation, metrics, multimodal references, continuation
references, embedded subagents, trials, verifier results, rewards, and
artifacts. Hosted sharing remains planned.

1. **Implemented.** Add a read-only Harbor job-directory source that maps job,
   dataset, task, trial, trajectory, verifier result, reward, timing, and
   artifact provenance into the existing canonical hierarchy. Do not infer
   missing outcomes or follow external references automatically.
2. **Implemented for bounded whole-source cohorts.** Add an explicit export step that packages one trajectory or bounded cohort
   with a versioned manifest, referenced public artifacts, source licenses, and
   redaction review. Export remains local and makes no upload by itself. Bundle
   v1 preserves inline artifacts and discloses, but does not copy, path-backed
   artifact files; trajectory selection and artifact embedding remain later
   extensions of the same schema.
3. Add a separate hosted share surface for public, unlisted, and later private
   read-only links. Upload is always explicit; the page uses the mobile reader,
   records the exact bundle digest, and provides expiration and deletion. The
   local reader never requires an account. Evaluate GitHub, Google, and X as
   hosted identity providers, choosing the smallest set that supports reliable
   stewardship and account recovery without coupling portable bundles to login.
4. Evaluate stable Harbor Hub job/trial links as an import boundary before
   duplicating Harbor storage or registry functions. Prefer reciprocal deep
   links and exact-version references over a competing runner or job manager.

Exit criteria: a user can open a complete local Harbor job, select a trial,
review its verifier evidence and artifacts, deliberately publish a scrubbed
bundle, and send a link that remains usable in a mobile browser.

### Phase B: curated benchmark showcases

Status: planned after Phase A.

1. Publish a small set of exact, pinned benchmark releases from Harbor,
   Hugging Face, or source repositories with clear ownership and licensing.
2. Pair each benchmark with representative public trajectories across outcomes,
   agents, and models; preserve the upstream artifact revision and run config.
3. Build task and trajectory pages on the same mobile reader and share-bundle
   contract. Start with a few widely used agent benchmarks rather than a broad
   leaderboard.
4. Show source-reported scores and transparent cohort summaries, never a new
   aggregate ranking whose comparability has not been established.

Exit criteria: every showcased result resolves to an immutable benchmark task,
environment or image reference, verifier revision, run configuration, and full
trajectory or an explicit disclosure that an artifact is unavailable.

### Phase C: benchmark catalog and provenance database

Status: later.

1. Introduce immutable benchmark, release, task, environment, seed, verifier,
   run, and trajectory records keyed by source revision and content digest.
2. Ingest metadata from supported upstream registries while leaving large
   source artifacts in their authoritative stores when possible.
3. Add search and filters for domain, harness, agent, model, outcome, artifact
   availability, license, and audit state.
4. Publish machine-readable clean-subset manifests and supersession history so
   downstream evaluations can pin exactly what they ran.

Exit criteria: a reported score or trajectory can be traced through exact
versioned task artifacts without relying on mutable names or copied metadata.

### Phase D: evidence claims, repairs, and stewardship

Status: later, after the catalog establishes stable identities.

1. Let contributors attach a coarse claim and free-form diagnosis to exact
   trajectory steps, verifier output, files, diffs, or environment evidence.
2. Support reproduce, dispute, repair proposal, rerun receipt, upstream-link,
   accepted, rejected, and superseded states without rewriting original traces.
3. Add benchmark-maintainer ownership and domain-scoped reviewer reputation.
   Award reputation for independently reproduced diagnoses, validated repairs,
   and upstream acceptance rather than raw votes.
4. Keep automated analysis visibly separate from human claims. Require exact
   producer, input digest, and evidence provenance for both.

Exit criteria: an accepted defect repair links the original task and
trajectories, grounded claim, patch, independent validation receipt, upstream
resolution, and replacement benchmark revision.

## Quality gates

Every change should keep these commands green:

```bash
make format
make test
make check
make build
```

Protocol changes require updated schemas, fixtures, and conformance tests. UI behavior changes require a browser-level test when the behavior is externally visible.
