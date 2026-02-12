# Go Coding Style Guide

> **Supplements**: [Effective Go](https://golang.org/doc/effective_go), `golangci-lint`, `gofmt`/`goimports`
> **Authority**: This document is the project's authoritative source for Go coding style.
> **ADR References**: [ADR-0003](../adr/ADR-0003-database-orm.md), [ADR-0013](../adr/ADR-0013-manual-di.md), [ADR-0016](../adr/ADR-0016-go-module-vanity-import.md)

---

## File & Function Size Limits

| Metric | Target | Hard Limit | Action |
|--------|--------|------------|--------|
| File size | 200–400 lines | 800 lines | Refactor: split into cohesive files |
| Function body | ≤50 lines | — | Split into helper functions |
| Nesting depth | ≤3 levels | 4 levels | Use early returns to flatten |

These limits are guidelines enforced during code review, not CI.

---

## Import Ordering

Use three groups separated by blank lines:

```go
import (
    // 1. Standard library
    "context"
    "fmt"
    "time"

    // 2. Third-party libraries
    "github.com/google/uuid"
    "golang.org/x/sync/errgroup"

    // 3. Project internal packages (vanity import, ADR-0016)
    "kv-shepherd.io/shepherd/ent"
    "kv-shepherd.io/shepherd/internal/service"
)
```

`goimports` handles this automatically when configured with `-local kv-shepherd.io/shepherd`.

---

## Struct Field Ordering

Order struct fields by lifecycle stage:

```go
type VMService struct {
    // 1. Dependencies (injected at construction)
    db       *ent.Client
    provider Provider
    queue    *river.Client

    // 2. Configuration (set at init, read-only after)
    timeout time.Duration
    logger  *slog.Logger

    // 3. Internal state (mutated at runtime)
    mu    sync.RWMutex
    cache map[uuid.UUID]*VM
}
```

---

## Naming Conventions

### Functions

```go
// ✅ Descriptive but concise
func CreateVirtualMachine(ctx context.Context, req *CreateVMRequest) (*VM, error)

// ✅ Short when context is clear (method on VMService)
func (s *VMService) Get(ctx context.Context, id uuid.UUID) (*VM, error)

// ❌ Too vague
func Create(ctx context.Context, r *Req) (*R, error)

// ❌ Needlessly verbose
func (s *VMService) GetVirtualMachineByIdentifier(ctx context.Context, id uuid.UUID) (*VM, error)
```

### Error Values

```go
// Package-level sentinel errors
var (
    ErrVMNotFound = errors.New("vm not found")
    ErrVMDeleted  = errors.New("vm already deleted")
)
```

---

## Error Handling

Use early returns to keep the main logic path at the left margin:

```go
// ✅ Good: early returns
func ProcessVM(vm *VM) error {
    if vm == nil {
        return errors.New("vm is nil")
    }
    if vm.Status == StatusDeleted {
        return ErrVMDeleted
    }

    // Main logic at left margin
    return nil
}

// ❌ Bad: deep nesting
func ProcessVM(vm *VM) error {
    if vm != nil {
        if vm.Status != StatusDeleted {
            // Main logic buried in nesting
        }
    }
    return errors.New("vm is nil")
}
```

---

## Logging (slog)

All logging **MUST** use structured `slog` calls:

```go
// ✅ Structured logging
s.logger.Info("creating vm",
    slog.String("name", req.Name),
    slog.String("namespace", req.Namespace),
    slog.String("tenant_id", req.TenantID.String()),
)

// ❌ Unstructured logging
log.Printf("Creating VM %s in namespace %s", req.Name, req.Namespace)
```

**FORBIDDEN**: Logging sensitive data (passwords, tokens, kubeconfig content).  
See [SECURITY_CODING.md §Logging & Error Safety](SECURITY_CODING.md) for details.

---

## Comment Standards

```go
// VMService provides virtual machine management functionality.
//
// It handles creation, updates, deletion of VMs and interacts
// with the KubeVirt provider layer.
type VMService struct { ... }

// CreateVM creates a new virtual machine.
//
// Parameters:
//   - ctx: Context for timeout and cancellation
//   - req: Creation request containing VM configuration
//
// Returns the created VM entity or an error if creation fails.
func (s *VMService) CreateVM(ctx context.Context, req *CreateVMRequest) (*ent.VM, error) {
    // ...
}
```

### TODO/FIXME Format

Issue reference is **required**. Bare comments without issue references are forbidden.

```go
// TODO(#123): implement retry logic for transient failures
// FIXME(#456): race condition when concurrent updates to same VM
```

---

## Style Prohibitions (Code Review Enforcement)

> **CI-enforced prohibitions**: See [CHECKLIST.md §Prohibited Patterns](CHECKLIST.md#prohibited-patterns)

These additional style rules are enforced during code review:

| Pattern | Rule |
|---------|------|
| `result, _ := doSomething()` | FORBIDDEN — handle all errors |
| `if condition { }` | FORBIDDEN — no empty blocks |
| `// TODO: fix later` | FORBIDDEN — must include issue number |
| Magic numbers (`if retries > 3`) | Use named constants: `const maxRetries = 3` |
| `log.Printf(...)` | Use `slog` structured logging |

---

## Performance Considerations

- Preallocate slice capacity when size is known: `make([]T, 0, expectedSize)`
- Avoid unnecessary memory allocations in hot paths
- Use `sync.Pool` for frequent small allocations
- Use appropriate concurrency patterns per [ADR-0031](../adr/ADR-0031-concurrency-and-worker-pool-standard.md)

---

## Testing Style

See [CONTRIBUTING.md §Testing](../../CONTRIBUTING.md#testing) for testing requirements and patterns.

---

## Related Documents

- [CONTRIBUTING.md](../../CONTRIBUTING.md) — Contribution workflow and commit standards
- [CHECKLIST.md §Prohibited Patterns](CHECKLIST.md#prohibited-patterns) — CI-enforced constraints
- [SECURITY_CODING.md](SECURITY_CODING.md) — Security coding guide
- [examples/](examples/) — Reference implementations
