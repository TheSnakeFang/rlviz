# Portable sharing

RLViz separates making a reviewed portable file from publishing it. The native
CLI can create an `.rlviz` bundle; neither the CLI nor the static browser
uploads it. The static browser can open an explicitly confirmed, digest-pinned
public HTTPS bundle. Hosting, deletion, and access control remain a separate
service boundary.

## Create a bundle

```bash
rlviz bundle create SOURCE \
  --out reviewed.rlviz \
  --title "Reviewed benchmark run" \
  --license CC-BY-4.0 \
  --reviewed \
  --redaction-confirmed
```

`SOURCE` may be any built-in file, a complete Harbor job directory, or the
output of an explicitly selected trusted adapter. The destination must be a new
`.rlviz` file. RLViz refuses to overwrite an existing path.

The confirmations are deliberately affirmative: `--reviewed` means the
trajectory content was inspected for publication, and
`--redaction-confirmed` means secrets and private data were removed. RLViz does
not claim that an automated scanner can make either judgment. `--license` is a
declared content license such as `CC-BY-4.0`, `MIT`, or `proprietary`.

An optional `--expires 2026-09-01T00:00:00Z` value records an advisory expiry in
the portable manifest. A copied file cannot delete itself. A future hosted
surface must enforce expiration and owner-initiated deletion server-side.

## Bundle v1 contract

A v1 file is a bounded ZIP containing exactly:

- `manifest.json`, using schema `rlviz.dev/bundle/v1`
- `trace.ndjson`, using the canonical `rlviz.dev/v1alpha1` record contract

The manifest records a title, creation time, license, basename-only source
name, source format and fingerprint, trace size and SHA-256, review and
redaction status, optional expiration, and explicit limitations. The archive
uses fixed entry metadata, so the same trace and fixed manifest metadata
produce the same bytes.

Opening a bundle rejects unknown entries, unsafe paths, duplicate entries,
oversized content, malformed manifests, size or digest mismatches, and invalid
canonical relationships. The browser retains its 32 MiB source ceiling.

## Artifact boundary

Inline text and JSON artifacts already present in canonical records travel with
the trace. Bundle v1 does not copy files referenced by artifact paths. Those
relative records remain useful provenance, but their content preview still
requires the original local source. The manifest and CLI output state how many
path-backed artifact files were excluded.

This is intentional: copying arbitrary job trees before there is a reviewed
artifact allowlist and redaction workflow would turn a safe trace export into a
high-risk archive tool.

## Open and verify

```bash
rlviz inspect reviewed.rlviz
rlviz open reviewed.rlviz
```

The same file can be dropped onto [rlviz.dev](https://rlviz.dev). Parsing,
digest verification, indexing, and rendering happen inside that browser tab.

## Link to a public bundle

A public catalog or static host can hand a bundle to the reader without
uploading it to RLViz:

```text
https://rlviz.dev/?bundle=https%3A%2F%2Fexample.org%2Frun.rlviz&sha256=FULL_64_HEX_DIGEST
```

RLViz displays the exact host, path, and digest without contacting the bundle
host. It makes one request only after the reader chooses **Verify and open**.
The request omits credentials and referrer information, refuses redirects and
URL credentials or query tokens, applies the browser size limit, and hashes the
complete `.rlviz` file before parsing it. The bundle's own manifest and trace
digest are then validated normally. The bundle host must permit the RLViz
origin through standard browser CORS headers.

This handoff is for immutable public files. Private links, expiring links,
uploads, deletion, and account-backed stewardship belong in the hosted
companion rather than the local-first viewer.
