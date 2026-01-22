# ADR-0016: Go Module Vanity Import Strategy

> **Status**: Accepted  
> **Date**: 2026-01-22  
> **Relates to**: Issue #10 (Project migration, rename, and Go Module strategy)

---

## Context

### Problem Statement

The project has migrated from `github.com/CloudPasture/kubevirt-shepherd` to `github.com/kv-shepherd/shepherd`. This raises a critical question about how to define the Go module path in `go.mod`.

Go module paths are **immutable** from the perspective of downstream users. Once users import a package using a specific path (e.g., `import "github.com/CloudPasture/kubevirt-shepherd/pkg/..."`), any change to that path requires them to update all their import statements. This creates a high barrier to future migrations, renames, or organizational changes.

### Options Considered

#### Option A: GitHub Direct Path

```go
// go.mod
module github.com/kv-shepherd/shepherd
```

**Pros:**
- Simple setup, no additional infrastructure
- Works immediately with `go get`

**Cons:**
- Locks the project to GitHub and the current organization name
- Future migrations (e.g., to a different org or self-hosted Git) would break all downstream users
- Any repository rename requires all users to update their imports

#### Option B: Vanity Import (Custom Domain)

```go
// go.mod
module kv-shepherd.io/shepherd
```

**Pros:**
- Decouples the module path from the Git hosting provider
- Enables painless migrations: changing the underlying repository only requires updating the vanity import configuration
- Follows the pattern used by major Go projects (e.g., `golang.org/x/...`, `k8s.io/...`)
- Professional appearance, brand consistency

**Cons:**
- Requires domain ownership and configuration
- Adds a dependency on the domain remaining active
- Slightly more complex initial setup

---

## Decision

**Use Vanity Import with our project domain**: `kv-shepherd.io/shepherd`

### Rationale

1. **Future-Proofing**: The project may need to migrate organizations (e.g., if accepted into a foundation) or rename the repository in the future. Vanity import insulates downstream users from these changes.

2. **Industry Best Practice**: Major Go projects use vanity imports:
   - `k8s.io/client-go` → GitHub: `kubernetes/client-go`
   - `golang.org/x/tools` → GitHub: `golang/tools`
   - `sigs.k8s.io/controller-runtime` → GitHub: `kubernetes-sigs/controller-runtime`

3. **Brand Consistency**: Using `kv-shepherd.io` aligns with our domain and creates a professional, unified identity.

4. **Zero Migration Cost**: We are at an early stage (pre-alpha) with no external users yet. Adopting vanity import now has zero cost, but adopting it later would require a major version bump.

---

## Implementation

### 1. go.mod Configuration

```go
// go.mod
module kv-shepherd.io/shepherd

go 1.25
```

### 2. Vanity Import Server Configuration

The vanity import requires a web server that responds to `go get` requests with the appropriate `<meta>` tag.

#### Option A: Static HTML (Recommended for Simplicity)

Host a static page at `https://kv-shepherd.io/shepherd?go-get=1` that returns:

```html
<!DOCTYPE html>
<html>
<head>
    <meta name="go-import" content="kv-shepherd.io/shepherd git https://github.com/kv-shepherd/shepherd">
    <meta name="go-source" content="kv-shepherd.io/shepherd https://github.com/kv-shepherd/shepherd https://github.com/kv-shepherd/shepherd/tree/main{/dir} https://github.com/kv-shepherd/shepherd/blob/main{/dir}/{file}#L{line}">
    <meta http-equiv="refresh" content="0; url=https://github.com/kv-shepherd/shepherd">
</head>
<body>
    Redirecting to <a href="https://github.com/kv-shepherd/shepherd">GitHub</a>...
</body>
</html>
```

#### Option B: Cloudflare Workers (Alternative)

Use a Cloudflare Worker to dynamically handle the `?go-get=1` query parameter.

#### Option C: vanity-imports Tool

Use a tool like [govanityurls](https://github.com/GoogleCloudPlatform/govanity) to generate the necessary redirects.

### 3. Cloudflare Pages Configuration (Recommended)

Since the domain is managed via Cloudflare, the simplest approach is:

1. Create a Cloudflare Pages project for `kv-shepherd.io`
2. Deploy a static site with the vanity import HTML
3. Optionally, host project documentation on the same domain

### 4. Import Path Examples

After adopting vanity import, all internal and example imports should use:

```go
import (
    "kv-shepherd.io/shepherd/internal/domain"
    "kv-shepherd.io/shepherd/internal/repository"
    "kv-shepherd.io/shepherd/pkg/apis/v1"
)
```

---

## Consequences

### Positive

- **Migration-proof**: Future organizational changes won't break downstream users
- **Professional branding**: Consistent use of project domain
- **Follows community standards**: Aligns with major Go projects

### Negative

- **Requires domain maintenance**: The domain `kv-shepherd.io` must remain active for the module path to resolve
- **Initial setup complexity**: Requires configuring vanity import server

### Neutral

- **No impact on development workflow**: `go get`, `go build`, etc. work identically
- **pkg.go.dev integration**: Works automatically once the module is published

---

## References

- [Go Modules Reference - Custom Import Paths](https://go.dev/ref/mod#vcs-find)
- [Kubernetes SIG Release - Vanity Imports](https://github.com/kubernetes/k8s.io/tree/main/k8s.io)
- [Google Cloud - govanityurls](https://github.com/GoogleCloudPlatform/govanity)
- Issue #10: Project migration, rename, and Go Module strategy

---

## Changelog

| Date | Change |
|------|--------|
| 2026-01-22 | Initial proposal, accepted |
