# Releasing RolloutViz

RolloutViz releases are built from versioned Git tags by
`.github/workflows/release.yml`. The workflow runs the full checks, builds four
native archives with GoReleaser, publishes SHA-256 checksums, creates GitHub
artifact attestations, and attaches a generated Homebrew formula.

## One-time setup

### Homebrew

1. Create the public repository `unlatch-ai/homebrew-tap` with a `main` branch.
2. Create a fine-grained GitHub token limited to that repository with
   `Contents: read and write`.
3. Store it in the RolloutViz repository secret `TAP_GITHUB_TOKEN`.

Without the secret, native release publication still succeeds and the formula
remains attached to the GitHub release. The Homebrew job reports that it skipped
the tap update.

### npm

The npm package is an optional installer for the same native archives. Follow
the bootstrap steps in [`packages/npm/README.md`](../packages/npm/README.md).
Until the npm trusted publisher is configured and the repository variable
`NPM_PUBLISH_ENABLED` equals `true`, the npm job is skipped.

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
   git tag -a v0.1.0 -m "rolloutviz v0.1.0"
   git push origin v0.1.0
   ```

5. Verify the GitHub release contains four archives, `checksums.txt`, and
   `rolloutviz.rb`, and that the attestation step passed.
6. On a clean machine, exercise one native archive, the curl installer, the
   Homebrew formula when enabled, and the npm package when enabled.

Do not reuse or move a published tag. Fix a broken release with a new patch
version. Checksums, package versions, and archive URLs are immutable release
contracts.
