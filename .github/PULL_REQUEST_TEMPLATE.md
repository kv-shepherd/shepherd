## Description

<!-- Describe your changes in detail -->

## Related Issue

<!-- Link to the issue this PR addresses (if applicable) -->
Fixes #

## Type of Change

<!-- Put an `x` in all the boxes that apply -->

- [ ] 🐛 Bug fix (non-breaking change which fixes an issue)
- [ ] ✨ New feature (non-breaking change which adds functionality)
- [ ] 💥 Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] 📚 Documentation update
- [ ] 🔧 Refactoring (no functional changes)
- [ ] 🧪 Test update

## Checklist

<!-- Put an `x` in all the boxes that apply -->

### Code Quality
- [ ] My code follows the project's coding standards
- [ ] I have run `golangci-lint run` and fixed all issues
- [ ] I have run `go test -race ./...` and all tests pass
- [ ] I have added tests that prove my fix/feature works

### Documentation
- [ ] I have updated relevant documentation
- [ ] I have updated the CHANGELOG.md (if applicable)
- [ ] Breaking changes are documented with migration guides

### Architecture
- [ ] My changes comply with existing [ADRs](docs/adr/)
- [ ] If this introduces a new architectural decision, I have created an ADR

### CI Checks
- [ ] No forbidden imports (GORM, Redis, Wire, naked goroutines)
- [ ] Transaction boundaries are respected
- [ ] K8s calls are outside DB transactions

## Screenshots (if applicable)

<!-- Add screenshots to help explain your changes -->

## Additional Notes

<!-- Add any additional notes for reviewers -->

---

By submitting this pull request, I confirm that my contribution is made under the terms of the Apache 2.0 license.
