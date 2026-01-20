# ADR-0002: Git Library Selection

> **Status**: Superseded  
> **Date**: 2026-01-14  
> **Superseded by**: [ADR-0007](./ADR-0007-template-storage.md)

---

## Supersession Notice

This ADR was superseded by ADR-0007, which replaced Git-based template storage with a pure database approach. The decision to use `go-git` is no longer applicable as templates are now stored in PostgreSQL.

**Reason for supersession**: Database storage provides better transactional consistency, simpler operations, and eliminates external Git server dependencies.

---

## Original Decision

Use `go-git` as the Git operations library.

| Item | Value |
|------|-------|
| Package | `github.com/go-git/go-git/v5` |
| Filesystem | `github.com/go-git/go-billy/v5` |

---

## Original Context

### Problem

Need to select a library for Git repository operations in Go. Main candidates:

1. **go-git**: Pure Go implementation
2. **git2go**: libgit2 Go bindings

### Constraints

- Avoid C dependencies (cross-platform compilation)
- Support basic Git operations (clone, pull, push, commit)
- Sufficient performance (config files typically KB-level)

---

## Options Considered

| Library | C Dependency | Cross-Platform | Performance | Maintenance |
|---------|-------------|----------------|-------------|-------------|
| `go-git` | ❌ Pure Go | ✅ | ⚠️ Slightly slower | ✅ Active |
| `git2go` | ✅ libgit2 | ❌ | ✅ | ✅ Active |

---

## Original Rationale

### 1. Pure Go Implementation

- No libgit2 installation required
- Simplified cross-platform compilation
- Static linking produces single binary

### 2. No CGO Issues

go-git completely avoids:
- CGO compilation problems
- Cross-platform compatibility issues

### 3. Sufficient Functionality

For config file synchronization:
- Clone/Pull/Push ✅
- Commit/Add ✅
- Authentication (SSH, HTTP) ✅

### 4. Extensible Storage

```go
// Supports custom storage backends
import (
    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/storage/memory"
    "github.com/go-git/go-billy/v5/memfs"
)

// In-memory repository (for testing)
r, _ := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
    URL: "https://github.com/example/repo",
})
```

---

## Code Examples (Historical)

### Clone Repository

```go
package git

import (
    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing/transport/http"
)

func CloneRepository(url, path, username, password string) (*git.Repository, error) {
    return git.PlainClone(path, false, &git.CloneOptions{
        URL: url,
        Auth: &http.BasicAuth{
            Username: username,
            Password: password,
        },
        Progress: os.Stdout,
    })
}
```

### Commit File

```go
func CommitFile(repo *git.Repository, filePath, message string) error {
    w, err := repo.Worktree()
    if err != nil {
        return err
    }
    
    // Add file
    _, err = w.Add(filePath)
    if err != nil {
        return err
    }
    
    // Commit
    _, err = w.Commit(message, &git.CommitOptions{
        Author: &object.Signature{
            Name:  "KubeVirt Hub",
            Email: "system@kubevirt-hub.local",
            When:  time.Now(),
        },
    })
    
    return err
}
```

---

## Consequences (Historical)

### Positive

- Zero C dependencies, simple deployment
- Perfect integration with Go toolchain
- Can use Go profiler for performance analysis

### Negative

- Slightly lower performance than libgit2 (negligible for this project)
- Some advanced Git features may not be supported

### Why Superseded

After further evaluation (see ADR-0007), we determined that:

1. **Git adds operational complexity** - Requires external Git server management
2. **Database is simpler** - PostgreSQL provides ACID guarantees natively
3. **No sync conflicts** - Database transactions eliminate merge conflicts
4. **Better query capability** - SQL filtering vs Git file traversal

---

## References

- [go-git Documentation](https://pkg.go.dev/github.com/go-git/go-git/v5)
- [go-git Examples](https://github.com/go-git/go-git/tree/master/_examples)
- [ADR-0007: Template Storage](./ADR-0007-template-storage.md)
