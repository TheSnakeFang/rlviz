# Releasing RLViz

RLViz releases are built from versioned Git tags by
`.github/workflows/release.yml`. The workflow runs the full checks, builds four
native archives with GoReleaser, publishes SHA-256 checksums, creates GitHub
artifact attestations, and attaches a generated Homebrew formula.

## One-time setup

### Homebrew

The public `unlatch-ai/homebrew-tap` repository polls the latest RLViz
release hourly and commits an updated attached formula with its repository-local
`GITHUB_TOKEN`. No cross-repository token is required. A maintainer can also run
the tap's `sync rlviz` workflow manually after a release.

### npm

The npm package is an optional installer for the same native archives. Its
trusted publisher is configured for `.github/workflows/release.yml`, and the
repository variable `NPM_PUBLISH_ENABLED` enables the job. Recovery and
re-bootstrap steps live in
[`packages/npm/README.md`](../packages/npm/README.md).

## Publish

1. Confirm the package version in `packages/npm/package.json` matches the release.
2. Run the local release gates:

   ```bash
   make check
   goreleaser release --snapshot --clean
   ```

3. Commit and push a clean `main` branch.
4. Create and push the tag:

   ```bash
   git tag -a v0.1.0 -m "rlviz v0.1.0"
   git push origin v0.1.0
   ```

5. Verify the GitHub release contains four archives, `checksums.txt`, and
   `rlviz.rb`, and that the attestation step passed.
6. On a clean machine, exercise one native archive, the curl installer, the
   Homebrew formula when enabled, and the npm package when enabled.

Do not reuse or move a published tag. Fix a broken release with a new patch
version. Checksums, package versions, and archive URLs are immutable release
contracts.
