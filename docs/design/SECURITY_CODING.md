# Security Coding Guide

> **Authority**: This document is the project's authoritative source for security coding practices.
> **ADR Reference**: [ADR-0019 §Security Baseline](../adr/ADR-0019-governance-security-baseline-controls.md)
> **Vulnerability Reporting**: See [SECURITY.md](../../SECURITY.md)

---

## Mandatory Security Checks

All code touching user data, authentication, or external system interactions **MUST** pass these checks during code review.

---

### 1. Secrets Management

**FORBIDDEN**: Hardcoded secrets in source code.

```go
// ❌ FORBIDDEN
const apiKey = "sk-xxxxx"
const dbPassword = "password123"
kubeconfig := "apiVersion: v1\nclusters:..."

// ✅ CORRECT
apiKey := os.Getenv("API_KEY")
if apiKey == "" {
    return errors.New("API_KEY not configured")
}

// Kubeconfig uses encrypted storage (AES-256-GCM)
kubeconfig, err := s.secretStore.GetKubeconfig(ctx, clusterID)
```

**Checklist**:
- [ ] No hardcoded API keys, tokens, or passwords
- [ ] All secrets via environment variables or encrypted storage
- [ ] `.env` files listed in `.gitignore`
- [ ] No secrets in Git history
- [ ] Kubeconfig uses AES-256-GCM encryption

---

### 2. Input Validation

**Rule**: All user inputs must have validation using whitelists (not blacklists).

```go
func (r *CreateVMRequest) Validate() error {
    if r.Name == "" {
        return errors.New("name is required")
    }
    if len(r.Name) > 63 {
        return errors.New("name too long")
    }
    if !validNameRegex.MatchString(r.Name) {
        return errors.New("name contains invalid characters")
    }
    return nil
}

// Always validate in handler layer
func (h *VMHandler) Create(ctx context.Context, req *CreateVMRequest) error {
    if err := req.Validate(); err != nil {
        return status.Errorf(codes.InvalidArgument, "validation: %v", err)
    }
    // Continue processing...
}
```

**Checklist**:
- [ ] All user inputs validated
- [ ] Uses whitelist validation (not blacklist)
- [ ] Error messages don't leak internal structure

---

### 3. SQL Injection Prevention

**Rule**: Always use Ent ORM or parameterized queries. Never concatenate user input into queries.

```go
// ❌ FORBIDDEN: String concatenation
query := fmt.Sprintf("SELECT * FROM vms WHERE name = '%s'", userName)

// ✅ CORRECT: Ent ORM (ADR-0003)
vms, err := client.VM.Query().
    Where(vm.Name(userName)).
    All(ctx)

// ✅ CORRECT: Parameterized query (if raw SQL needed, whitelisted dirs only)
rows, err := db.Query("SELECT * FROM vms WHERE name = $1", userName)
```

---

### 4. Tenant Isolation

**Rule**: ALL data queries MUST include tenant ID filter. No cross-tenant data access.

```go
func (s *VMService) GetVM(ctx context.Context, id uuid.UUID) (*ent.VM, error) {
    tenantID := auth.TenantIDFromContext(ctx)

    vm, err := s.db.VM.Query().
        Where(
            vm.ID(id),
            vm.TenantID(tenantID), // REQUIRED — never omit
        ).
        Only(ctx)

    if err != nil {
        return nil, fmt.Errorf("get vm: %w", err)
    }
    return vm, nil
}
```

**Checklist**:
- [ ] All API endpoints have authentication
- [ ] Sensitive operations have authorization checks
- [ ] Tenant isolation in every query
- [ ] Follows least privilege principle

---

### 5. Logging & Error Safety

**Rule**: Never expose internal details to users. Log details server-side only.

```go
// ❌ FORBIDDEN: Logging sensitive data
log.Printf("User login: %s, password: %s", email, password)
log.Printf("Kubeconfig: %s", kubeconfig)

// ✅ CORRECT: Redact sensitive data
s.logger.Info("user login", slog.String("email", email))
// Don't log passwords or kubeconfig content

// ❌ FORBIDDEN: Exposing internal details to user
return fmt.Errorf("database error: %s, query: %s", err, query)

// ✅ CORRECT: Generic user-facing error + detailed server log
s.logger.Error("internal error", slog.String("error", err.Error()))
return errors.New("internal error, please try again")
```

---

### 6. Kubernetes RBAC

**Rule**: Service accounts must use minimum required permissions (ADR-0019).

```go
// Create minimal permission Role
role := &rbacv1.Role{
    Rules: []rbacv1.PolicyRule{
        {
            APIGroups: []string{"kubevirt.io"},
            Resources: []string{"virtualmachines"},
            Verbs:     []string{"get", "list", "watch"},
        },
    },
}
```

- No wildcard permissions (`*`) except bootstrap role
- Bootstrap role must be disabled after initial setup
- RFC 1035 naming for all platform-managed resources

---

## Code Review Security Checklist

Every code review **MUST** verify:

- [ ] No hardcoded secrets
- [ ] All inputs validated (whitelist approach)
- [ ] Uses Ent ORM or parameterized queries
- [ ] Proper authentication and authorization
- [ ] Logs don't contain sensitive information
- [ ] Error messages don't leak internal details
- [ ] Tenant isolation correctly implemented
- [ ] K8s RBAC uses minimum permissions

---

## Severity Response Table

| Level | Description | Required Response |
|-------|-------------|-------------------|
| **CRITICAL** | Remote code execution, data breach | Fix immediately, deploy hotfix |
| **HIGH** | Auth bypass, privilege escalation | Fix within 7 days |
| **MEDIUM** | Information disclosure, DoS | Fix in next release |
| **LOW** | Minor issues | Address per roadmap |

---

## References

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [ADR-0019 §Security Baseline](../adr/ADR-0019-governance-security-baseline-controls.md)
- [SECURITY.md](../../SECURITY.md)
- [CODING_STYLE.md §Logging](CODING_STYLE.md)
