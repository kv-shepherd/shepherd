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

The release process is fully automated via [release-please](https://github.com/googleapis/release-please) once the repository secret `RELEASE_PLEASE_TOKEN` is configured with a token that can open release PRs.

> For alpha/beta/rc releases, keep the `release-please` prerelease settings and manifest baseline aligned with the intended prerelease track. Use a one-time bootstrap input only when you need to pin the first prerelease version.

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
   - PR title: `chore(main): release 0.1.0-alpha.1` when bootstrapping the first alpha release
   - Release PR commits are generated with `Signed-off-by` when `signoff: true` is enabled

3. **Merge the Release PR** → release-please creates:
   - Git tag (for example `v0.1.0-alpha.1` or `v0.1.0`)
   - GitHub Release with auto-generated changelog

4. **Bootstrap a first prerelease if needed**
   - Open the `Release Please` workflow manually with `workflow_dispatch`
   - Provide `release_as=0.1.0-alpha.1` for the initial alpha bootstrap
   - Subsequent releases continue automatically from the generated manifest baseline

5. **Tag push triggers artifact build** (`.github/workflows/release.yml`):
   - Go binaries (linux/amd64, linux/arm64) → GitHub Release assets
   - Docker images → `ghcr.io/kv-shepherd/shepherd-server`, `shepherd-web`
   - Cosign keyless signatures for all container images
   - SHA-256 checksums

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
| Go Binaries | GitHub Release assets | `shepherd` + `seed` for linux/amd64, linux/arm64 |
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
