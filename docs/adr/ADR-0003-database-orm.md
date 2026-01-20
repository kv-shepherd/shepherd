# ADR-0003: Database ORM Selection

> **Status**: Accepted  
> **Date**: 2026-01-14

---

## Decision

Use **PostgreSQL + Ent ORM + Atlas migration tool** as the data layer technology stack.

| Component | Selection |
|-----------|-----------|
| Database | PostgreSQL 18.x |
| ORM | Ent |
| Driver | pgx/v5 |
| Migration Tool | Atlas |

> **Version Reference**: Specific versions are defined in [DEPENDENCIES.md](../design/DEPENDENCIES.md) (single source of truth)

---

## Context

### Problem

Need to select a database access approach for Go. Considerations:

1. Type safety
2. Dynamic query capability
3. Migration tool support
4. Developer-friendly API

### Constraints

- Support complex dynamic queries (filtering, sorting, pagination)
- Support transactional DDL (reversible migrations)
- Compile-time type safety to reduce runtime errors

---

## Options Considered

| Option | Type | Type Safety | Dynamic Query | Migration |
|--------|------|-------------|---------------|-----------|
| GORM + MySQL | ORM | ⭐⭐ | ⭐⭐⭐ | golang-migrate |
| sqlx + MySQL | Extended | ⭐ | ⭐⭐ | golang-migrate |
| **Ent + PostgreSQL** | ORM | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | Atlas |

### Why Not GORM + MySQL

| Issue | Description |
|-------|-------------|
| GORM type-unsafe | `map[string]interface{}` updates, reflection magic |
| MySQL DDL non-transactional | Failed migrations cannot be rolled back |
| Dynamic query cumbersome | Manual string concatenation required |
| CI enforcement unreliable | AST checks can be bypassed |

---

## Ent + PostgreSQL Advantages

### 1. Compile-Time Type Safety

```go
// ❌ GORM: Field name error discovered at runtime
db.Where("stauts = ?", "running").Find(&vms)  // typo: stauts → status

// ✅ Ent: Compile-time error
client.VM.Query().
    Where(vm.StatusEQ(vm.StatusRunning)).  // Status is strongly typed enum
    All(ctx)
```

### 2. Type-Safe Dynamic Queries (Predicate Composition)

```go
// Dynamic filter conditions
var predicates []predicate.VM

if filter.Cluster != "" {
    predicates = append(predicates, vm.ClusterEQ(filter.Cluster))
}
if filter.Status != "" {
    predicates = append(predicates, vm.StatusEQ(vm.Status(filter.Status)))
}
if filter.CPUMin > 0 {
    predicates = append(predicates, vm.CPUGTE(filter.CPUMin))
}

// Query
vms, err := client.VM.Query().
    Where(vm.And(predicates...)).
    Order(ent.Desc(vm.FieldCreatedAt)).
    Limit(pageSize).
    Offset(offset).
    All(ctx)
```

### 3. PostgreSQL Transactional DDL

```sql
-- PostgreSQL supports transactional DDL
BEGIN;
  ALTER TABLE vms ADD COLUMN cpu_cores INT;
  -- If subsequent operations fail...
ROLLBACK;  -- Can safely rollback!

-- MySQL DDL auto-commits, cannot rollback
```

### 4. Atlas Declarative Migrations

```hcl
# atlas.hcl
env "local" {
  src = "ent://ent/schema"
  dev = "docker://postgres/18/dev?search_path=public"
  
  migration {
    dir = "file://migrations"
  }
}
```

```bash
# Auto-generate migrations
atlas migrate diff --env local

# Apply migrations (with rollback support)
atlas migrate apply --env local
```

### 5. JSONB Indexing

```sql
-- PostgreSQL JSONB supports indexing
CREATE INDEX idx_vm_spec_cpu ON vm_records ((spec_snapshot->>'cpu_cores'));

-- Query uses index
SELECT * FROM vm_records WHERE spec_snapshot->>'cpu_cores' > '4';
```

---

## Code Examples

### Schema Definition

```go
// ent/schema/vm.go

package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type VM struct {
    ent.Schema
}

func (VM) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").NotEmpty(),
        field.String("namespace").NotEmpty(),
        field.String("cluster_name").NotEmpty(),
        field.Enum("status").
            Values("PENDING", "RUNNING", "STOPPED", "FAILED", "DELETED").
            Default("PENDING"),
        field.String("created_by").NotEmpty(),
        field.JSON("spec_snapshot", map[string]interface{}{}).Optional(),
        field.String("k8s_uid").Optional().Nillable(),
        field.Bool("k8s_exists").Default(true),
        field.String("error_message").Optional().Nillable(),
        field.String("idempotency_key").Optional().Nillable().Unique(),
    }
}

func (VM) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("cluster_name", "namespace", "name").Unique(),
        index.Fields("status"),
        index.Fields("created_by"),
    }
}
```

### Query Example

```go
// Compile-time type-safe queries
func (r *VMRepository) ListByFilter(ctx context.Context, filter VMFilter) ([]*ent.VM, error) {
    query := r.client.VM.Query()
    
    // Type-safe dynamic conditions
    if filter.ClusterName != "" {
        query = query.Where(vm.ClusterNameEQ(filter.ClusterName))
    }
    if filter.Namespace != "" {
        query = query.Where(vm.NamespaceEQ(filter.Namespace))
    }
    if filter.Status != "" {
        query = query.Where(vm.StatusEQ(vm.Status(filter.Status)))
    }
    
    return query.
        Order(ent.Desc(vm.FieldCreatedAt)).
        Limit(filter.PageSize).
        Offset(filter.Offset()).
        All(ctx)
}
```

### Transaction Management

```go
// Reusable transaction helper
func WithTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
    tx, err := client.Tx(ctx)
    if err != nil {
        return err
    }
    defer func() {
        if v := recover(); v != nil {
            tx.Rollback()
            panic(v)
        }
    }()
    if err := fn(tx); err != nil {
        if rerr := tx.Rollback(); rerr != nil {
            err = fmt.Errorf("%w: rolling back transaction: %v", err, rerr)
        }
        return err
    }
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("committing transaction: %w", err)
    }
    return nil
}
```

---

## Consequences

### Positive

- ✅ Compile-time type safety reduces runtime errors
- ✅ Predicate composition for type-safe dynamic queries
- ✅ PostgreSQL transactional DDL enables reversible migrations
- ✅ Atlas declarative migrations deeply integrated with Ent
- ✅ JSONB indexing support

### Negative

- 🟡 Learning curve for Ent code generation pattern
- 🟡 Schema changes require code regeneration

### Mitigation

- CI checks for Ent codegen sync (`check_ent_codegen.go`)
- Forbidden GORM imports (`check_forbidden_imports.go`)

---

## References

- [Ent Documentation](https://entgo.io/docs/getting-started)
- [Atlas Documentation](https://atlasgo.io/getting-started)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
