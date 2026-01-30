---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"
date: 2026-01-30
deciders: []
consulted: []
informed: []
---

# ADR-0025: Bootstrap Secrets Auto-Generation and Persistence

> **Review Period**: Until 2026-02-01
> **Discussion**: [Issue #XX](https://github.com/kv-shepherd/shepherd/issues/XX)
> **Amends**: [ADR-0018](./ADR-0018-instance-size-abstraction.md) (bootstrap config guidance)

---

## Context and Problem Statement

Deployment-time secrets (`ENCRYPTION_KEY`, `SESSION_SECRET`) are security-critical but create
operational friction when required before first boot. For V1, we prioritize usability and
define a safe default that avoids blocking initial startup. Key rotation is explicitly
deferred to a future RFC. Secrets are stored in PostgreSQL with minimal access privileges.

## Decision Drivers

* Strong security defaults with encrypted-at-rest secrets
* Zero-friction first boot for users
* Defer rotation complexity to a later RFC
* Keep secrets accessible to the application without restart

## Considered Options

* **Option 1**: Require operators to provide all secrets before startup
* **Option 2**: Allow missing secrets; run with ephemeral in-memory keys
* **Option 3**: Auto-generate secrets on first boot and persist (no rotation in V1)

## Decision Outcome

**Chosen option**: "Auto-generate secrets on first boot and persist (no rotation in V1)",
because it balances security with usability and avoids unsafe ephemeral keys.

### Consequences

* ✅ Good, because first boot succeeds without pre-provisioned secrets
* ✅ Good, because secrets are durable and available across restarts
* 🟡 Neutral, because bootstrap needs a storage location for generated secrets
* ❌ Bad, because rotation is deferred (mitigation: RFC-0016)

### Confirmation

* Unit tests: secrets generated only when missing, persisted once
* Integration tests: boot with missing secrets produces stable persisted keys
* Security checks: secrets never logged; encryption works with generated keys

---

## Pros and Cons of the Options

### Option 1: Require operators to provide all secrets

* ✅ Good, because key management is explicit
* ❌ Bad, because increases setup friction and delays first boot

### Option 2: Allow missing secrets (ephemeral in-memory)

* ✅ Good, because easiest to start
* ❌ Bad, because secrets rotate on restart and break encrypted data and sessions

### Option 3: Auto-generate and persist (no rotation in V1)

* ✅ Good, because secure by default with low friction
* ❌ Bad, because rotation is deferred

---

## More Information

### Related Decisions

* [ADR-0018](./ADR-0018-instance-size-abstraction.md) - Deployment-time config overview
* [ADR-0019](./ADR-0019-governance-security-baseline-controls.md) - Secrets handling baseline
* [RFC-0016](../rfc/RFC-0016-key-rotation.md) - Key rotation (future work)

### References

* RFC 7518 (JWA) key length guidance: https://www.rfc-editor.org/rfc/rfc7518
* OWASP Secrets Management: https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html

### Implementation Notes

* Generate 32-byte random keys using a CSPRNG
* Persist generated secrets in PostgreSQL on first boot
* Load secrets from DB into memory on startup; do not auto-rotate in V1
* Precedence: external key (KMS/secret manager) > env vars > DB-generated
* If an external/env key is introduced later, require an explicit re-encryption step
* Store secrets in `system_secrets` table (single row or per key)
* Access control: only application DB role can read/write; no admin UI/API exposure

**Proposed table shape** (non-normative):
- `id`, `key_name`, `key_value`, `source`, `created_at`, `updated_at`

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-30 | @codex | Initial draft |
