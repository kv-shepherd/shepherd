# ADR-0013: Manual Dependency Injection

> **Status**: Accepted  
> **Date**: 2026-01-18

---

## Decision

Adopt **strict manual dependency injection**, deprecating Wire (goforj/wire).

---

## Context

The project initially planned to use [goforj/wire](https://github.com/goforj/wire) (community fork of Google Wire) for compile-time dependency injection.

### Problems with Original Approach

1. **Supply chain risk**: goforj/wire is a community fork with uncertain long-term maintenance
2. **Code generation complexity**: Requires running `wire` command to generate `wire_gen.go`, increasing CI complexity
3. **Debugging difficulty**: Troubleshooting requires examining generated code, not source
4. **Potential misuse**: Wire DSL can be misused, causing generation failures

---

## Core Standards

1. **Composition Root**: All dependency assembly centralized in `internal/app/bootstrap.go`
2. **Constructor Injection**: Dependencies explicitly declared via `New*()` functions
3. **Layered Construction**: Infrastructure → Repository → Service → UseCase → Handler
4. **CI Enforcement**: `check_manual_di.sh` script prohibits instantiation outside `internal/app/`

---

## Implementation Example

```go
// internal/app/bootstrap.go
// 🚨 Composition Root - All dependency assembly must be centralized here

func Bootstrap(ctx context.Context, cfg *config.Config) (*Application, error) {
    // Layer 1: Infrastructure
    pool, _ := infrastructure.NewPgxPool(ctx, cfg.Database.DSN)
    entClient, _ := infrastructure.NewEntClient(pool)
    
    // Layer 2: Repository
    vmRepo := repository.NewVMRepository(entClient)
    
    // Layer 3: Service
    vmService := service.NewVMService(vmRepo)
    
    // Layer 4: Handler
    vmHandler := handlers.NewVMHandler(vmService)
    
    return &Application{...}, nil
}
```

---

## Wire vs Manual DI Comparison

| Dimension | Wire (goforj) | Strict Manual DI |
|-----------|--------------|------------------|
| Boilerplate code | Less | More (acceptable trade-off) |
| Compile check | Requires running `wire` | `go build` checks directly |
| Debugging difficulty | Medium (need wire_gen.go) | Low (what you see is what you get) |
| Supply chain risk | 🔴 High (fork maintenance risk) | 🟢 Zero (only Go compiler) |
| Potential misuse | DSL can be misused | Compiler catches errors directly |

---

## Rationale

Wire's primary value proposition (reducing boilerplate code) is significantly diminished when balanced against:

1. **Zero supply chain dependencies** - No external code generation tools required
2. **Explicit dependency graph** - The bootstrap file documents all dependencies
3. **Simplified CI** - No additional generation step needed
4. **Direct debugging** - All code is visible without generation artifacts

---

## Consequences

### Positive

- ✅ Zero supply chain risk
- ✅ Direct compilation without code generation step
- ✅ Explicit, reviewable dependency graph
- ✅ Simpler CI pipeline

### Negative

- 🟡 More boilerplate code in bootstrap.go
- 🟡 Changes in dependencies require manual updates

### Mitigation

- Bootstrap file is typically modified infrequently
- Boilerplate is straightforward and easy to maintain

---

## Impact

- Remove `internal/wire/` directory
- Remove `check_wire_bypass.go` CI check
- Add `check_manual_di.sh` CI check
- Update all Phase documents

---

## References

- [Wire Deprecation Details](./archived/wire-dependency-injection-archived.md)
- [Uber Go Style Guide - Dependency Injection](https://github.com/uber-go/guide/blob/master/style.md)
