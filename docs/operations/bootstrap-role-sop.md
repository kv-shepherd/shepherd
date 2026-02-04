# Bootstrap Role Security SOP

> **Status**: Required for production deployment  
> **Reference**: [ADR-0019](../adr/ADR-0019-governance-security-baseline-controls.md)  
> **Related**: [master-flow.md §Stage 1.A](../design/interaction-flows/master-flow.md)

---

## Overview

The `role-bootstrap` with `platform:admin` permission is a **temporary role** that exists ONLY during initial platform setup. This document defines the standard operating procedure for managing and disabling this role.

> ⚠️ **ADR-0019 Compliance**: Wildcard permissions (`*:*`) are **PROHIBITED** for all roles. The bootstrap role uses `platform:admin` (explicit super-admin permission) and MUST be disabled after initialization.

---

## Bootstrap Role Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Bootstrap Role Lifecycle                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Stage 1: Platform Initialization                                            │
│  ├── Database seed creates role-bootstrap with platform:admin permission    │
│  ├── First user account created with bootstrap role                         │
│  └── Bootstrap user performs initial configuration                          │
│                                                                              │
│  Stage 2: Admin Transfer (MANDATORY)                                         │
│  ├── Bootstrap user creates first PlatformAdmin user                        │
│  ├── PlatformAdmin user changes bootstrap password                          │
│  └── System automatically disables bootstrap role (trigger)                 │
│                                                                              │
│  Stage 3: Normal Operation                                                   │
│  ├── role-bootstrap has no active bindings                                   │
│  ├── All users use explicit permission roles                                 │
│  └── Audit log records bootstrap role deactivation                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Procedure

### Step 1: Complete Initial Setup

Using the bootstrap account:

1. Create at least one user with `role-platform-admin`
2. Configure required system settings:
   - Default cluster selection (if applicable)
   - Authentication provider (OIDC/LDAP)
   - Initial template import

### Step 2: Transfer Admin Privileges

1. Log in as the newly created PlatformAdmin user
2. Verify admin functionality works correctly
3. Navigate to RBAC management and verify permissions

### Step 3: Disable Bootstrap Role

**Option A: Automatic Deactivation (Recommended)**

The system automatically disables the bootstrap role when:
- The bootstrap user's password is changed by a PlatformAdmin
- OR 24 hours have elapsed since platform initialization

**Option B: Manual Deactivation**

If automatic deactivation fails, execute manually:

```sql
-- Verify bootstrap role is still active
SELECT * FROM role_bindings WHERE role_id = 'role-bootstrap';

-- Remove all bootstrap role bindings
DELETE FROM role_bindings WHERE role_id = 'role-bootstrap';

-- Record in audit log
INSERT INTO audit_logs (action, actor, target_type, target_id, details, created_at)
VALUES (
  'SECURITY',
  'system',
  'role',
  'role-bootstrap',
  '{"action": "manual_deactivation", "reason": "SOP compliance"}',
  NOW()
);
```

### Step 4: Verify Deactivation

```sql
-- Should return 0 rows
SELECT COUNT(*) FROM role_bindings WHERE role_id = 'role-bootstrap';

-- Verify audit log entry exists
SELECT * FROM audit_logs 
WHERE target_id = 'role-bootstrap' 
AND action = 'SECURITY'
ORDER BY created_at DESC
LIMIT 1;
```

---

## Security Verification Checklist

Run this checklist before going to production:

| Check | SQL Query | Expected Result |
|-------|-----------|-----------------|
| No bootstrap bindings | `SELECT COUNT(*) FROM role_bindings WHERE role_id = 'role-bootstrap'` | `0` |
| No active bootstrap permissions | `SELECT * FROM role_permissions rp JOIN role_bindings rb ON rp.role_id = rb.role_id WHERE rp.role_id = 'role-bootstrap'` | `0 rows` |
| Audit entry exists | `SELECT * FROM audit_logs WHERE target_id = 'role-bootstrap' AND action = 'SECURITY'` | `≥1 row` |
| PlatformAdmin uses platform:admin | `SELECT * FROM role_permissions WHERE role_id = 'role-platform-admin'` | Single `platform:admin` permission |

---

## Emergency Access (Break-Glass)

If emergency access is required after bootstrap role deactivation:

1. **DO NOT** re-enable the bootstrap role or create any `*:*` wildcard permissions
2. Use database-level access with proper authorization and audit trail
3. Create a new PlatformAdmin user directly in database if needed:

```sql
-- Emergency: Create emergency admin (requires database admin access)
-- This action MUST be recorded in change management system
INSERT INTO users (id, email, name, password_hash, created_at)
VALUES (
  gen_random_uuid(),
  'emergency-admin@company.com',
  'Emergency Admin',
  -- Use bcrypt hash of temporary password
  '$2a$10$...',
  NOW()
);

-- Assign platform:admin explicitly (NOT *:*)
INSERT INTO role_bindings (user_id, role_id, created_at)
VALUES (
  (SELECT id FROM users WHERE email = 'emergency-admin@company.com'),
  'role-platform-admin',
  NOW()
);
```

4. After emergency is resolved, disable the emergency account
5. Document the incident in security audit log

---

## Periodic Audit (Quarterly)

As per ADR-0019, perform quarterly verification:

1. Run Security Verification Checklist
2. Review all `role-platform-admin` bindings (should be minimal)
3. Verify no wildcard permissions exist in any role
4. Document audit results

---

## Related Documents

- [ADR-0019: Governance Security Baseline Controls](../adr/ADR-0019-governance-security-baseline-controls.md)
- [master-flow.md: Platform Initialization](../design/interaction-flows/master-flow.md)
- [04-governance.md: RBAC Model](../design/phases/04-governance.md)
