# Release Process

> This document describes the release process for KubeVirt Shepherd.

---

## Versioning

KubeVirt Shepherd follows [Semantic Versioning 2.0.0](https://semver.org/):

- **Major** (X.0.0): Breaking API changes
- **Minor** (0.X.0): New features, backward compatible
- **Patch** (0.0.X): Bug fixes, security patches

### Pre-release Versions

| Stage | Format | Example |
|-------|--------|---------|
| Alpha | `vX.Y.Z-alpha.N` | `v0.1.0-alpha.1` |
| Beta | `vX.Y.Z-beta.N` | `v0.1.0-beta.1` |
| Release Candidate | `vX.Y.Z-rc.N` | `v1.0.0-rc.1` |

---

## Release Cadence

| Type | Frequency | Description |
|------|-----------|-------------|
| **Patch** | As needed | Security fixes, critical bugs |
| **Minor** | ~2-3 months | New features |
| **Major** | ~12 months | Breaking changes |

---

## Release Checklist

### Pre-Release

- [ ] All CI checks pass on `main` branch
- [ ] Unit test coverage ≥ 60%
- [ ] No critical security vulnerabilities (Dependabot/Snyk)
- [ ] CHANGELOG.md auto-updated by release-please (review in Release PR)
- [ ] Documentation updated for new features
- [ ] ADR/RFC status updated if applicable

### Release Process

The release process is fully automated via [release-please](https://github.com/googleapis/release-please) and the repository `GITHUB_TOKEN`. The repository or organization must grant GitHub Actions `Read and write permissions` and allow GitHub Actions to create pull requests. The release workflow also needs `actions: write` so it can dispatch the artifact publishing workflow after creating a release.

> KubeVirt Shepherd uses the `prerelease` versioning strategy for ongoing alpha work. Once a prerelease line is bootstrapped, routine `feat:` and `fix:` commits advance `vX.Y.Z-alpha.N` within the same base version until maintainers intentionally promote the line.

1. **Write Conventional Commits** on the `main` branch
   ```
   feat: add multi-cluster VM migration
   fix: correct RBAC permission check for viewers
   feat!: redesign approval workflow API (BREAKING CHANGE)
   docs: update production deployment guide
   ```

2. **Release PR is auto-created** by release-please
   - Updates `CHANGELOG.md` with categorized changes
   - Bumps version in `.release-please-manifest.json`
   - Routine alpha releases advance within the same base version, for example `v0.1.1-alpha.1` → `v0.1.1-alpha.2` → `v0.1.1-alpha.3`
   - Release PRs should be authored by `github-actions[bot]`
   - Release PR commits include a `Signed-off-by` trailer when `signoff` is configured with a valid `Name <email>` identity

3. **Merge the Release PR** → release-please creates:
   - Git tag (for example `v0.1.0-alpha.1` or `v0.1.0`)
   - GitHub Release with auto-generated changelog

4. **Bootstrap or promote intentionally**
   - For a one-time bootstrap pin, merge a small commit whose body includes `Release-As: x.y.z`
   - The `workflow_dispatch` `release_as` input is useful for reruns or operator-driven promotion
   - To keep iterating on the same alpha line, do nothing special; the prerelease strategy will increment only `alpha.N`
   - To promote a mature line to a stable release such as `v0.1.2`, use one intentional `Release-As: 0.1.2` override or the workflow dispatch input with `release_as=0.1.2`

5. **Artifact build is dispatched automatically** (`.github/workflows/release.yml`):
   - Go runtime archives (linux/amd64, linux/arm64) → GitHub Release assets
     with `shepherd`, `seed`, `atlas`, and bundled Atlas migrations
   - Docker images → `ghcr.io/kv-shepherd/shepherd-server`, `shepherd-web`
   - Cosign keyless signatures for all container images
   - SHA-256 checksums
   - This dispatch is explicit because tags and releases created by the repository `GITHUB_TOKEN` do not fan out into downstream workflows automatically

6. **Post-Release**
   - Announce release (GitHub Discussions)
   - Verify container image signatures: `cosign verify ghcr.io/kv-shepherd/shepherd-server:v0.1.0-alpha.1`

---

## Hotfix Process

For critical security or bug fixes:

1. Create branch from release tag
2. Apply fix and create new patch tag
3. Cherry-pick to `main` if applicable

---

## Release Artifacts

| Artifact | Location | Description |
|----------|----------|-------------|
| Go Runtime Archives | GitHub Release assets | `shepherd` + `seed` + `atlas` + bundled migrations for linux/amd64, linux/arm64 |
| Server Image | `ghcr.io/kv-shepherd/shepherd-server:vX.Y.Z` | Go backend (distroless, multi-arch) |
| Web Image | `ghcr.io/kv-shepherd/shepherd-web:vX.Y.Z` | Next.js frontend (node:22-alpine, multi-arch) |
| Cosign Signatures | Stored in ghcr.io | Keyless OIDC signatures for all container images |
| Checksums | GitHub Release assets | SHA-256 checksums for Go binaries |

---

## Deprecation Policy

- **Deprecated features**: Announced at least one minor release before removal
- **Breaking changes**: Documented in CHANGELOG with migration guide

---

## References

- [CHANGELOG.md](../CHANGELOG.md)
- [CONTRIBUTING.md](../CONTRIBUTING.md)
- [SECURITY.md](../SECURITY.md)
