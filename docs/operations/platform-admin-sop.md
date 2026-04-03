# Platform Administrator Bootstrap and Review SOP

> **Status**: Active operational guidance  
> **Reference**: [ADR-0019](../adr/ADR-0019-governance-security-baseline-controls.md), [ADR-0053](../adr/ADR-0053-prelaunch-rbac-baseline-cleanup.md)  
> **Related**: [master-flow.md §Stage 1.5 / 2.A](../design/interaction-flows/master-flow.md)

---

## Overview

Shepherd no longer seeds a dedicated `role-bootstrap`.

Initial platform setup is performed by the seeded default admin account, and
that account is reconciled onto the canonical `role-platform-admin` built-in
role. From that point forward, ongoing administration uses ordinary
`PlatformAdmin` role bindings and standard audit controls.

This SOP defines:

* how to complete the initial handoff from the seeded default admin
* how to verify `PlatformAdmin` assignments remain narrow
* how to perform emergency recovery without inventing temporary wildcard roles

---

## Initial Setup Flow

### Step 1: Use the seeded default admin only for first-entry tasks

The seeded default admin exists to let the first operator:

1. log in successfully
2. change the default password
3. verify clusters, templates, and auth configuration
4. create at least one durable `PlatformAdmin` assignment for a named operator

### Step 2: Create the durable platform admin assignment

Before handing the system to normal operation, create at least one named user
with the built-in `PlatformAdmin` role.

Recommended pattern:

* at least two named platform administrators
* no shared long-lived admin identity
* all bindings created through ordinary RBAC APIs or audited database
  operations

### Step 3: Reduce reliance on the seeded default admin

Once named platform administrators can log in and manage the platform:

1. verify their access works end-to-end
2. rotate the seeded default admin password if the account must remain
   available for break-glass
3. otherwise disable or tightly control the seeded default admin account per
   local operations policy

There is no separate bootstrap-role deactivation step, because there is no
bootstrap role in the current baseline.

---

## Security Verification Checklist

Run this checklist before go-live and during quarterly review:

| Check | Query / Action | Expected Result |
|------|------|------|
| Only canonical built-ins exist | `SELECT id, name FROM roles WHERE built_in = true ORDER BY id;` | `role-platform-admin`, `role-approval-admin`, `role-development-engineer`, `role-test-engineer`, `role-system-operator`, `role-viewer` |
| Platform admin role is explicit | `SELECT permissions FROM roles WHERE id = 'role-platform-admin';` | only `platform:admin` |
| No compatibility permissions remain in stored roles | inspect `roles.permissions` | no `cluster:manage`, `template:manage`, `auth_provider:manage` |
| Named platform admins exist | review bindings for `role-platform-admin` | minimal named operators only |
| Seeded default admin is controlled | account review | password rotated or account disabled/controlled |

---

## Emergency Access

If emergency platform access is required:

1. do **not** reintroduce a bootstrap-only role
2. do **not** create wildcard permissions such as `*:*`
3. create or restore a `PlatformAdmin` binding through audited administrative
   action

Example recovery flow:

1. create a temporary named emergency user
2. bind `role-platform-admin`
3. record the change in ticketing / audit systems
4. remove the temporary binding when the incident is resolved

---

## Related Documents

* [ADR-0019: Governance Security Baseline Controls](../adr/ADR-0019-governance-security-baseline-controls.md)
* [ADR-0053: Pre-Launch RBAC Baseline Cleanup](../adr/ADR-0053-prelaunch-rbac-baseline-cleanup.md)
* [master-flow.md](../design/interaction-flows/master-flow.md)
