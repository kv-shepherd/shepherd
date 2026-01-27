---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-01-27
deciders: []  # GitHub usernames of decision makers
consulted: []  # Subject-matter experts consulted (two-way communication)
informed: []  # Stakeholders kept up-to-date (one-way communication)
---

# ADR-0019: Governance Security Baseline Controls

> **Review Period**: Until 2026-01-29 (48-hour minimum)  
> **Discussion**: [Issue #25](https://github.com/kv-shepherd/shepherd/issues/25)  
> **Amends**: [ADR-0015 §6](./ADR-0015-governance-model-v2.md), [ADR-0015 §16](./ADR-0015-governance-model-v2.md), [ADR-0015 §22](./ADR-0015-governance-model-v2.md)

---

## Context and Problem Statement

We need a single, enforceable governance baseline for naming, RBAC, and audit logging that remains valid even as implementation details evolve. These controls must be explicit, conservative, and auditable without modifying accepted ADRs.

## Decision Drivers

* Minimize governance risk by using the most conservative safe defaults.
* Maintain ADR immutability while still updating current guidance.
* Provide executable security requirements (not just principles).
* Align with widely accepted industry and Kubernetes/OWASP practices.

## Considered Options

* **Option 1**: Keep rules only inside existing ADRs and documents (no new ADR).
* **Option 2**: Create a new ADR that defines governance security baselines and amends prior ADRs.
* **Option 3**: Defer to implementation-only controls without architectural decision.

## Decision Outcome

**Chosen option**: "Option 2", because it preserves ADR immutability while defining a single source of truth for baseline controls and concrete requirements.

### Consequences

* ✅ Good, because governance rules are centralized and auditable.
* ✅ Good, because future changes can amend this ADR instead of editing accepted ones.
* 🟡 Neutral, because some existing docs will require amendment blocks after acceptance.
* ❌ Bad, because an extra ADR adds review overhead (mitigated by clear scope).

### Confirmation

* Review checklist for naming, RBAC, and logging compliance before merge.
* Architecture review verifies amendment blocks added to affected ADRs after acceptance.
* Security review validates log redaction and access control policy in code or config.

### Acceptance Checklist (Execution Tasks)

Upon acceptance, perform the following updates (do not modify original ADR content; append amendment blocks only):

* Append "Amendments by Subsequent ADRs" to ADR-0015 for:
  * §6 Audit trail
  * §16 Naming rules
  * §22 Platform RBAC model
* Update governance/design documents to align with this ADR:
  * `docs/design/phases/01-contracts.md`
  * `docs/design/phases/04-governance.md`
* Add or update code review checklist items to ensure:
  * name validation enforces RFC 1035-style rules
  * RBAC roles avoid wildcards except bootstrap role
  * logs redact sensitive fields and enforce access control

---

## Baseline Controls (Normative)

### 1. Naming Policy (Most Conservative)

**Policy**: All platform-managed logical names (System, Service, Namespace, VM name components) MUST follow RFC 1035-based label rules with additional conservative constraints:

* lowercase letters, digits, and hyphen only (`a-z`, `0-9`, `-`)
* MUST start with a letter (`a-z`)
* MUST end with a letter or digit
* MUST NOT contain consecutive hyphens (`--`) ※

> ※ **Note**: The consecutive hyphen prohibition is an **additional conservative constraint** beyond RFC 1035. RFC 5891 reserves `--` at positions 3-4 for Punycode (e.g., `xn--`); this policy extends the restriction to all positions for simplicity and future-proofing.

**Length Constraints**: See [ADR-0015 §16](./ADR-0015-governance-model-v2.md) for component length limits (System/Service/Namespace: max 15 characters each).

**Reserved Names**: Names that conflict with platform-reserved or system-level identifiers SHOULD be avoided. Examples include but are not limited to: `default`, `system`, `admin`, `root`, `internal`, or any names prefixed with `kube-` or `kubevirt-shepherd-`. The implementation MAY maintain a configurable deny-list.

**Rationale**: This is the most conservative compatible naming policy across Kubernetes resource types and avoids reliance on feature gates such as `RelaxedServiceNameValidation`.

### 2. Platform RBAC Least Privilege

**Policy**:

* Wildcard permissions (e.g., `*:*`) are prohibited for all roles except a single, built-in bootstrap role.
* The bootstrap role (`role-platform-admin` or equivalent) is intended **exclusively** for platform initialization and MUST:
  * be narrowly distributed (assigned only to platform operators during initial setup),
  * be **disabled or its bindings removed** after platform initialization is complete,
  * be subject to **periodic audit** (at least quarterly) to verify no active bindings exist in production,
  * remain revocable via administrative action at any time.
* If the bootstrap role must be temporarily re-enabled for maintenance, the activation and deactivation MUST be recorded in the audit log.
* All other roles MUST use explicit verbs and resources (no wildcards).

**Rationale**: Minimizes accidental privilege escalation and limits future resource exposure. The manual disable + audit approach balances security with operational simplicity for internal governance platforms.

### 3. Audit Logging and Sensitive Data Controls

**Policy**:

* Logs and audit records MUST NOT store the following data in plaintext:
  * passwords or initial credentials
  * access tokens, session IDs, refresh tokens
  * encryption keys or key material
  * database connection strings
  * external system secrets (webhooks, client secrets)
* If any of the above must be represented, use one of:
  * redaction (e.g., fixed mask such as `***REDACTED***`),
  * hashing (non-reversible, for correlation only),
  * encryption at rest with restricted access.
* Log access MUST be restricted to authorized roles and audited.
* Log entries MUST be protected against tampering (append-only or equivalent integrity control).

**Integrity Control Implementation Guidance**:

| Control | Implementation |
|---------|----------------|
| Append-only storage | PostgreSQL: use `INSERT`-only audit table with no `UPDATE`/`DELETE` grants for application roles |
| Database-level protection | Revoke `DELETE` and `TRUNCATE` privileges on audit tables from all non-admin roles |
| Application-level enforcement | Audit logger service MUST NOT expose delete or update methods |
| Retention policy | Soft archive via `archived_at` timestamp; physical deletion only via scheduled admin job with separate audit trail |

**Rationale**: Prevents logs from becoming a high-impact data leakage vector and preserves forensic integrity.

---

## Pros and Cons of the Options

### Option 1: No new ADR (status quo)

* ✅ Good, because no extra process overhead
* ❌ Bad, because accepted ADRs remain ambiguous and cannot be updated cleanly

### Option 2: New ADR for baseline controls

* ✅ Good, because governance rules are centralized without altering accepted ADRs
* ✅ Good, because each baseline control is explicit and testable
* 🟡 Neutral, because it requires post-acceptance amendment blocks in older ADRs

### Option 3: Implementation-only controls

* ✅ Good, because fastest to implement
* ❌ Bad, because architectural intent is undocumented and not reviewable

---

## More Information

### Related Decisions

* [ADR-0015](./ADR-0015-governance-model-v2.md) - Governance model and RBAC structure

### References

* Kubernetes object naming rules and RFC 1035/1123 label constraints: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/
* Kubernetes RBAC good practices (least privilege, avoid wildcards): https://kubernetes.io/docs/concepts/security/rbac-good-practices/
* OWASP logging guidance on sensitive data exclusion and log protection: https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html

### Implementation Notes

* After acceptance, append an "Amendments by Subsequent ADRs" block to ADR-0015 referencing this ADR for:
  * §6 Audit trail
  * §16 Naming rules
  * §22 Platform RBAC model
* Update governance/design docs to align with these baseline controls.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-27 | @jindyzhao | **Clarification**: RFC 1035 naming policy now explicitly notes that consecutive hyphen prohibition is an additional conservative constraint beyond RFC 1035, with RFC 5891 rationale |
| 2026-01-27 | @jindyzhao | Initial draft |
