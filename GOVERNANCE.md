# Governance

This document describes the governance model for the KubeVirt Shepherd project.

## Principles

KubeVirt Shepherd follows these governance principles:

- **Open**: The project is open source under the Apache 2.0 license
- **Transparent**: All discussions happen in public channels
- **Meritocratic**: Contributions and community involvement drive advancement
- **Welcoming**: We embrace contributors from all backgrounds

## Project Roles

### Contributors

Anyone who contributes to the project (code, documentation, issues, reviews, etc.) is a contributor.

**Responsibilities:**
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md)
- Follow the [Contributing Guidelines](CONTRIBUTING.md)
- Respect the project's [ADRs](docs/adr/) and coding standards

### Maintainers

Maintainers are contributors with write access to the repository. They are responsible for:

- Reviewing and merging pull requests
- Triaging issues
- Participating in architecture decisions (ADRs)
- Mentoring new contributors
- Ensuring project quality and stability

**Current Maintainers:**

| Name | GitHub | Focus Area |
|------|--------|------------|
| *To be announced* | - | Core |

> **Note**: As a new project, the maintainer list will be populated as the community grows.
> See [MAINTAINERS.md](MAINTAINERS.md) for detailed maintainer roles and responsibilities.

### Becoming a Maintainer

Contributors who have demonstrated the following may be nominated for maintainership:

1. **Sustained contributions** over at least 3 months
2. **Quality contributions**: Multiple merged PRs with good code quality
3. **Community engagement**: Helpful in issues, reviews, and discussions
4. **Technical understanding**: Familiarity with the project's architecture and ADRs

**Process:**
1. Existing maintainers nominate a contributor
2. Discussion period of 1 week
3. Lazy consensus (no objections) or majority vote if needed
4. Announcement and access granted

### Emeritus Maintainers

Maintainers who are no longer actively contributing may move to emeritus status. Emeritus maintainers:
- Are recognized for their past contributions
- May return to active status upon renewed activity
- Do not have merge permissions

## Decision Making

### Day-to-Day Decisions

- **Minor changes** (bug fixes, documentation): Single maintainer approval
- **Feature additions**: At least one maintainer approval, preferably two
- **Dependency updates**: Verify compatibility, single maintainer approval

### Architectural Decisions

Significant architectural changes require an **Architecture Decision Record (ADR)**:

1. Proposer creates an ADR following the [template](docs/adr/README.md)
2. Discussion period of at least 1 week
3. Maintainer consensus required (lazy consensus or 2/3 majority)
4. ADR is merged with `Accepted` status

See [docs/adr/](docs/adr/) for existing decisions.

### Breaking Changes

Changes that break backward compatibility require:

1. An ADR documenting the breaking change
2. At least 2/3 maintainer approval
3. Migration guide in documentation
4. Deprecation notice in prior release (when applicable)

## Conflict Resolution

1. **Technical disagreements**: Discuss in the issue/PR; maintainers decide by consensus
2. **Interpersonal conflicts**: Refer to [Code of Conduct](CODE_OF_CONDUCT.md)
3. **Escalation**: If consensus cannot be reached, a vote among maintainers decides

## Meetings

As the project grows, we may establish:

- Regular community meetings (announced via GitHub Discussions)
- Office hours for contributor questions
- Design review sessions for major features

## Communication Channels

| Channel | Purpose |
|---------|---------|
| [GitHub Issues](https://github.com/kv-shepherd/shepherd/issues) | Bug reports, feature requests |
| [GitHub Discussions](https://github.com/kv-shepherd/shepherd/discussions) | General questions, ideas |
| [GitHub PRs](https://github.com/kv-shepherd/shepherd/pulls) | Code contributions |

## Changes to Governance

Changes to this governance document require:

1. A pull request with the proposed changes
2. Discussion period of at least 2 weeks
3. Approval by 2/3 of active maintainers

---

*This governance model is adapted from open source best practices.*
