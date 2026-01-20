# ADR-0011: K8s Resource Submission Strategy - Server-Side Apply + Unstructured

> **Status**: Accepted  
> **Date**: 2026-01-16  
> **Related**: [ADR-0007](./ADR-0007-template-storage.md) (Template Storage)

---

## Decision

### Adopt: Option B - Unstructured + Server-Side Apply

**Core Principle**: Backend is a **"YAML porter"**, not a **"Struct assembly factory"**.

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Resource Submission Flow                         │
│                                                                      │
│  DB Template → text/template render → YAML string → Unstructured    │
│                                          ↓                           │
│                               Kubernetes API Server                  │
│                                          ↓                           │
│                               Server-Side Apply (SSA)                │
│                               FieldOwner: kubevirt-shepherd          │
│                               Force: true                            │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Context

### Options Considered

| Option | Description |
|--------|-------------|
| **A. Typed Struct + Create/Update** | Use `kubevirt.io/api` typed struct, call `client.Create()` |
| **B. Unstructured + SSA** | Use `unstructured.Unstructured`, call `client.Patch(Apply)` |
| **C. Hybrid** | Render with typed struct for validation, submit with Unstructured + SSA |

---

## Rationale

### 1. Version Decoupling (Core Advantage)

**Problem Scenario**:

```
KubeVirt v1.8 releases, adds field spec.domain.memory.hugepages.pageSize

→ Typed Struct approach:
  1. Update go.mod dependency
  2. Modify Go code
  3. Recompile and deploy
  Time: 1+ day

→ Unstructured approach:
  1. Ops modifies template YAML in database
  2. Immediate effect
  Time: 5 minutes
```

### 2. KubeVirt is External CRD

Per industry best practices:

| CRD Type | Recommended Client | Reason |
|----------|-------------------|--------|
| **Own CRD** | Typed Client | Type-safe, self-maintained |
| **External CRD** | Unstructured | Avoid version lock-in |

KubeVirt is **external CRD** (not defined by this project), using Unstructured follows best practices.

### 3. SSA Governance Advantages

| Feature | Create/Update | SSA (Patch Apply) |
|---------|---------------|-------------------|
| **Idempotency** | ❌ Create fails need checking | ✅ Naturally idempotent |
| **Concurrency conflict** | ❌ Needs Get-Modify-Put | ✅ Server-side merge |
| **Field ownership** | ❌ Cannot express | ✅ FieldOwner explicit |
| **Self-healing** | ❌ None | ✅ Force overwrites |

---

## SSA Strategy Configuration

### FieldOwner

```go
const FieldOwner = "kubevirt-shepherd"
```

### Force Policy

```go
patchOpts := []client.PatchOption{
    client.ForceOwnership,               // Force overwrite conflicts
    client.FieldOwner("kubevirt-shepherd"),
}
```

| Force | Behavior | Use Case |
|-------|----------|----------|
| **true** | Force overwrite other FieldOwner's changes | ✅ Governance platform (platform is Source of Truth) |
| **false** | Apply fails on conflict | Scenarios allowing manual intervention |

> **Platform Positioning**: `kubevirt-shepherd` is the governance platform, must ensure K8s resources match platform state.
> 
> If user manually modifies VM via `kubectl`, platform will **force restore** on next Apply.

---

## Implementation

### Core Apply Function

```go
// internal/provider/ssa_applier.go

type SSAApplier struct {
    k8sClient client.Client
    decoder   *yaml.Serializer
}

// ApplyYAML submits YAML string to K8s via SSA
func (a *SSAApplier) ApplyYAML(ctx context.Context, yamlData string) error {
    // 1. YAML → Unstructured (completely decoupled from typed struct)
    obj := &unstructured.Unstructured{}
    _, _, err := a.decoder.Decode([]byte(yamlData), nil, obj)
    if err != nil {
        return fmt.Errorf("yaml decode failed: %w", err)
    }

    // 2. SSA Patch
    patchOpts := []client.PatchOption{
        client.ForceOwnership,
        client.FieldOwner("kubevirt-shepherd"),
    }

    return a.k8sClient.Patch(ctx, obj, client.Apply, patchOpts...)
}

// DryRunApply preview mode (doesn't actually create resources)
func (a *SSAApplier) DryRunApply(ctx context.Context, yamlData string) error {
    // ... same as above with client.DryRunAll added
}
```

---

## Type Safety Guarantee

**Question**: After abandoning typed struct, how to catch field typos?

**Answer**: Rely on **Server-Side Dry-Run**

```go
func (s *TemplateService) ValidateBeforeSave(ctx context.Context, content string) error {
    // 1. Go Template syntax check
    // 2. Render with mock data
    // 3. K8s Server-Side Dry-Run validation (more authoritative than Go compiler)
    return s.ssaApplier.DryRunApply(ctx, yamlData)
}
```

---

## Consequences

### Positive

- ✅ **Version decoupling**: KubeVirt upgrades don't require Go code changes
- ✅ **Hot update capability**: Template changes take effect immediately
- ✅ **Clear governance authority**: FieldOwner declares platform ownership
- ✅ **Idempotency**: SSA naturally idempotent, simplifies retry logic
- ✅ **Self-healing**: Force mode ensures platform is Source of Truth

### Negative

- 🟡 **Lost compile-time checks**: Field typos only discovered at runtime (mitigated by Dry-Run)
- 🟡 **Reduced IDE support**: Unstructured has no auto-completion
- 🟡 **Learning curve**: Developers need to understand SSA and managedFields

---

## References

- [Kubernetes Server-Side Apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/)
- [ADR-0007: Template Storage](./ADR-0007-template-storage.md)
