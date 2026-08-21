# Changelog

## 0.4.0

- Add an explicit-consent browser handoff for immutable public HTTPS `.rlviz`
  bundles, pinned by the complete file SHA-256 and fetched without credentials,
  redirects, or referrer information.
- Add reviewed portable `.rlviz` bundles with provenance, license, redaction,
  digest, and advisory-expiration metadata.
- Add native, read-only Harbor job ingestion for ATIF trajectories, CTRF
  verifier evidence, rewards, usage, exceptions, and path-backed artifacts.
- Add a mobile trajectory reader for outcome, story, evidence, and exact detail
  views without reducing the underlying trajectory model.
- Replace browser mode swaps with the serializable workspace rack: a
  collapsible collection rail, two-band trajectory stage, bottom console,
  per-lane view state, reference pins, resizable persisted seams, arrangement
  deep links, and browser-integrated jumplist history.
- Add the local-first rollout instrument UI with Browse, Read, stage-aligned
  Compare, attention ordering, caterpillar projections, fidelity controls, and
  local verdict tags.
- Add a keyboard-first TUI that reads the same indexed trajectories and honors
  the shared semantic palette and `NO_COLOR`.
- Add the `rlviz init` first-run wizard with explicit, create-only agent skill
  installation.
- Add a deterministic synthetic gallery covering a coding-agent bugfix,
  research run, and mixed-outcome rollout cohort.
- Add validated light, dark, and high-contrast palette support.
- Add the static documentation site at [rlviz.dev](https://rlviz.dev), deployed
  through Vercel.
- Known limitation: lane depth currently changes navigation state only;
  distinct per-depth representations are planned for workspace build phase 2.
