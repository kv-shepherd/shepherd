# Contributing to KubeVirt Shepherd

Thank you for your interest in contributing to KubeVirt Shepherd! This document provides guidelines and instructions for contributing.

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## How to Contribute

### Reporting Issues

- Check existing [issues](https://github.com/kv-shepherd/shepherd/issues) before creating a new one
- Use the issue templates when available
- Include relevant details: version, environment, steps to reproduce

### Submitting Pull Requests

1. **Fork** the repository
2. **Create a feature branch** from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```
3. **Make your changes** following our coding standards
4. **Run tests and linters**:
   ```bash
   go test -race ./...
   golangci-lint run
   ```
5. **Commit** with clear, descriptive messages
6. **Push** to your fork and create a Pull Request

### Commit Message Guidelines

Follow [Conventional Commits](https://www.conventionalcommits.org/) with the **50/72 rule**:

```
<type>(<scope>): <description>     ← 50 chars max, imperative, no period
                                    ← blank line
[body: explain what and why]        ← wrap at 72 chars
                                    ← blank line
[footer: issue refs, sign-off]      ← Refs #N or Closes #N
Signed-off-by: Your Name <email>
```

### Commit Message Rules

| Rule | Requirement |
|------|-------------|
| Subject line | ≤50 characters, imperative mood, no period |
| Blank line | Required between subject and body |
| Body | Wrap at 72 characters, explain *what* and *why* |
| Footer | Issue references, DCO sign-off |

### Types

`feat` | `fix` | `docs` | `style` | `refactor` | `test` | `chore` | `perf` | `ci`

### Issue Reference Keywords

| Keyword | Effect | When to Use |
|---------|--------|-------------|
| `Refs #N` | Links only | Partial work, ongoing discussion |
| `Part of #N` | Links only | Multi-PR issue |
| `Closes #N` | **Closes on merge** | Only when PR fully resolves issue |
| `Fixes #N` | **Closes on merge** | Bug fixes that fully resolve |

> ⚠️ **IMPORTANT**: Use `Refs #N` for work-in-progress. Only use `Closes/Fixes` when the PR **completely** resolves the issue.

### Example

```
feat(provider): add KubeVirt snapshot support

Implements VM snapshot functionality using the KubeVirt VolumeSnapshot
API. This enables point-in-time recovery for VirtualMachine instances.

Key changes:
- Add SnapshotProvider interface
- Implement KubeVirt VolumeSnapshot adapter
- Add snapshot lifecycle management

Closes #123
Signed-off-by: Your Name <email@example.com>
```

## Development Setup

### Prerequisites

- Go 1.25+
- PostgreSQL 18+
- Docker (for testcontainers)
- Access to a Kubernetes cluster with KubeVirt installed (for integration tests)

### Getting Started

```bash
# Clone your fork
git clone git@github.com:YOUR_USERNAME/shepherd.git
cd kubevirt-shepherd

# Install dependencies
go mod download

# Generate Ent code
go generate ./ent/...

# Run unit tests
go test ./...

# Run linter
golangci-lint run
```

### Running Locally

```bash
# Start PostgreSQL (using Docker)
docker run -d --name postgres-dev \
  -e POSTGRES_USER=shepherd \
  -e POSTGRES_PASSWORD=shepherd \
  -e POSTGRES_DB=kubevirt_shepherd \
  -p 5432:5432 postgres:18

# Apply migrations
atlas migrate apply --env local

# Run the server
go run cmd/server/main.go
```

## Coding Standards

### Architecture Decisions

All code must comply with our [Architecture Decision Records (ADRs)](docs/adr/):

| ADR | Key Requirement |
|-----|-----------------|
| ADR-0003 | Use Ent ORM only (no GORM) |
| ADR-0006 | All writes through River Queue |
| ADR-0012 | sqlc only in whitelisted directories |
| ADR-0013 | Manual dependency injection |

### CI Checks

All PRs must pass these checks:

| Check | Description |
|-------|-------------|
| `golangci-lint` | Static analysis |
| `go test -race` | Unit tests with race detection |
| `check_naked_goroutine.go` | No naked goroutines |
| `check_sqlc_usage.sh` | sqlc scope enforcement |

See [docs/design/ci/README.md](docs/design/ci/README.md) for the complete list.

### Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` and `goimports`
- Keep functions focused and small
- Document exported types and functions
- Write tests for new functionality

## Documentation

- Update relevant documentation when changing functionality
- ADR changes require discussion and approval
- Keep examples in `docs/design/examples/` synchronized with actual patterns

## Testing

### Unit Tests

```bash
go test ./...
```

### Integration Tests (requires Docker)

```bash
go test -tags=integration ./...
```

### Test Coverage

Aim for ≥60% test coverage on new code.

## Review Process

1. All PRs require at least one maintainer approval
2. CI checks must pass
3. Documentation must be updated if applicable
4. Breaking changes require ADR update
5. All review comments must be resolved before merge

### Labels

Use labels to categorize your PR:

| Label | Description |
|-------|-------------|
| `kind/feature` | New feature |
| `kind/bug` | Bug fix |
| `kind/documentation` | Documentation update |
| `kind/cleanup` | Code cleanup or refactoring |
| `area/core` | Core functionality |
| `area/api` | API changes |
| `area/provider` | KubeVirt provider |
| `good first issue` | Suitable for new contributors |

### Draft PRs

Open a **Draft PR** early to:
- Get early feedback on your approach
- Allow others to track your progress
- Prevent duplicate work

Mark as "Ready for Review" when complete.

### Handling Review Feedback

When addressing review comments:

```bash
# Make changes, then use fixup commits
git commit --fixup=<commit-hash>

# Before final merge, squash fixups
git rebase --autosquash -i main
```

> 💡 Avoid commit messages like "Address review comments" - squash them into meaningful commits before merge.

## Getting Help

- Open a [GitHub Discussion](https://github.com/kv-shepherd/shepherd/discussions)
- Check existing [documentation](docs/)
- Review related [ADRs](docs/adr/) for design context

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).

---

Thank you for contributing to KubeVirt Shepherd! 🐑
