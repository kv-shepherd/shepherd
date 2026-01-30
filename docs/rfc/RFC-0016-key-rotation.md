# RFC-0016: Secret Key Rotation

> **Status**: Proposed  
> **Priority**: P2  
> **Trigger**: Compliance requirement or operator request for periodic key rotation

## Problem

V1 defers key rotation to reduce complexity. As deployments mature, operators will
require periodic rotation of `ENCRYPTION_KEY` and `SESSION_SECRET` without downtime
or data loss.

## Proposed Solution

Introduce a keyring model with versioned keys:

- Store a list of key versions in DB (`active`, `retired`) with creation timestamps
- Encrypt new data with `active` key
- Decrypt by trying `active` then `retired` keys
- Provide a rotation workflow that:
  1. Generates a new key version
  2. Marks it active
  3. Re-encrypts existing sensitive fields in background
  4. Retires old keys after migration

## Trade-offs

### Pros
- No downtime rotation
- Backward compatibility during migration
- Clear auditability of key changes

### Cons
- Additional complexity in crypto handling
- Requires background migration job

## Implementation Notes

- Add `key_version` metadata to encrypted fields or store in an encryption envelope
- Ensure secrets are never logged
- Provide an admin-only rotation endpoint or CLI

## References

- ADR-0025: Bootstrap Secrets Auto-Generation and Persistence
- OWASP Secrets Management: https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html
