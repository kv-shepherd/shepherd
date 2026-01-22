# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial project documentation structure
- 14 Architecture Decision Records (ADRs)
- 15 Request for Comments (RFCs) for future features
- 5 Implementation Phase specifications (Phase 0-4)
- Reference implementations and code examples
- CI check scripts for code quality enforcement
- Community governance files (CODE_OF_CONDUCT, GOVERNANCE, MAINTAINERS, ADOPTERS)

### Architecture Decisions

- **ADR-0001**: Use KubeVirt official client-go for type-safe VM operations
- **ADR-0003**: Use Ent as primary ORM with Atlas migrations
- **ADR-0006**: Unified async model with River Queue
- **ADR-0012**: Ent + sqlc hybrid transaction strategy for atomic operations
- **ADR-0013**: Strict manual dependency injection (no Wire)
- **ADR-0014**: Runtime KubeVirt capability detection

### Dependencies

- Go 1.25.6
- PostgreSQL 18.x
- River Queue v0.30.0
- Ent v0.14.5
- KubeVirt client-go v1.7.0

---

## Version History

> This section will be populated as releases are made.

[Unreleased]: https://github.com/kv-shepherd/shepherd/compare/main...HEAD
