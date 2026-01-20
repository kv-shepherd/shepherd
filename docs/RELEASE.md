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
- [ ] CHANGELOG.md updated with new version section
- [ ] Documentation updated for new features
- [ ] ADR/RFC status updated if applicable

### Release Process

1. **Create Release Branch** (for minor/major releases)
   ```bash
   git checkout main
   git pull origin main
   git checkout -b release/vX.Y.Z
   ```

2. **Update Version**
   - Update version in code (if applicable)
   - Update CHANGELOG.md

3. **Create Tag**
   ```bash
   git tag -a vX.Y.Z -m "Release vX.Y.Z"
   git push origin vX.Y.Z
   ```

4. **GitHub Actions Automation**
   - Triggered by tag push
   - Builds container images
   - Runs full test suite
   - Creates GitHub Release with changelog

5. **Post-Release**
   - Merge release branch back to `main` (if applicable)
   - Announce release (GitHub Discussions)

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
| Container Image | `ghcr.io/cloudpasture/kubevirt-shepherd:vX.Y.Z` | Multi-arch image |
| SBOM | GitHub Release assets | Software Bill of Materials |
| Checksums | GitHub Release assets | SHA256 checksums |

---

## Deprecation Policy

- **Deprecated features**: Announced at least one minor release before removal
- **Breaking changes**: Documented in CHANGELOG with migration guide

---

## References

- [CHANGELOG.md](../CHANGELOG.md)
- [CONTRIBUTING.md](../CONTRIBUTING.md)
- [SECURITY.md](../SECURITY.md)
