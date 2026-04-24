# Releasing

Two release channels, both fully automated by GitHub Actions.

## Rolling prerelease (`latest-prerelease`)

Automatic. Every push to `main` triggers `.github/workflows/release.yml`, which:

1. Runs `go test ./...`
2. Cross-compiles 6 binaries (darwin/linux/windows × amd64/arm64) with `main.buildVersion=latest-prerelease`.
3. Replaces the `latest-prerelease` tag and GitHub release with the new artifacts + `checksums.txt`.

No action required. This is what `scripts/install-release.sh --tag latest-prerelease` pulls.

## Stable release (`vX.Y.Z`)

Triggered by pushing a tag matching `v*`. From a clean, up-to-date `main`:

```bash
git checkout main && git pull
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

`.github/workflows/tagged-release.yml` then:

1. Runs `go test ./...`
2. Builds the same 6 binaries with `main.buildVersion=X.Y.Z` (the `v` is stripped).
3. Creates a GitHub release titled `Release vX.Y.Z` with notes auto-generated from merged PRs since the previous tag. If a release already exists for that tag it's deleted and recreated (the workflow is idempotent).

Default users of `scripts/install-release.sh` (and the README's `curl | bash` one-liner) resolve `latest` via the GitHub API and receive the newest stable `v*` release.

### Picking the version

Semver, bumped from the previous stable tag:

- Patch (`v1.0.0` → `v1.0.1`): bug fixes only.
- Minor (`v1.0.0` → `v1.1.0`): backward-compatible features.
- Major (`v1.0.0` → `v2.0.0`): breaking changes to flags, commands, or env vars.

### Previewing release notes

```bash
gh api repos/wherobots/wbc-cli/releases/generate-notes \
  -f tag_name=vX.Y.Z -f previous_tag_name=<previous-tag>
```

### If something goes wrong

- **Workflow failed mid-run:** re-run it from the Actions tab. The "Create GitHub release" step deletes any existing release for the tag, so reruns are safe.
- **Wrong tag pushed:** delete the remote tag (`git push origin :refs/tags/vX.Y.Z`) and the release (`gh release delete vX.Y.Z`), then push the correct tag.
