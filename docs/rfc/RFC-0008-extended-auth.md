# RFC-0008: Extended Authentication Providers

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Enterprise requires MFA/SAML 2.0 or active token revocation

---

## Scope Clarification

> ⚠️ **Note**: Basic OIDC/LDAP integration has been accepted as part of [ADR-0015: Governance Model V2](../adr/ADR-0015-governance-model-v2.md) §22 (Authentication & RBAC Strategy).
>
> **This RFC now covers only advanced authentication features not included in ADR-0015:**
>
> | Feature | ADR-0015 Status | This RFC Status |
> |---------|-----------------|-----------------|
> | OIDC Integration | ✅ Accepted | N/A |
> | LDAP Integration | ✅ Accepted | N/A |
> | Guided IdP Configuration | ✅ Accepted | N/A |
> | **Multi-factor Authentication** | ❌ Not covered | **Deferred** |
> | **SAML 2.0 Support** | ❌ Not covered | **Deferred** |
> | **Advanced Session Management** | ❌ Not covered | **Deferred** |

---

## Problem

Enterprise deployments may require advanced authentication features beyond basic OIDC/LDAP:
- Multi-factor authentication (MFA/2FA)
- SAML 2.0 support (for legacy enterprise IdPs)
- Advanced session management (concurrent session limits, forced logout, token revocation)

---

## Proposed Solution

### Provider Interface

```go
type AuthProvider interface {
    Authenticate(ctx context.Context, credentials Credentials) (*User, error)
    ValidateToken(ctx context.Context, token string) (*Claims, error)
    RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)
}

// Implementations:
// - LocalAuthProvider (current)
// - LDAPAuthProvider
// - OIDCAuthProvider
// - SAMLAuthProvider
```

### Configuration

```yaml
auth:
  provider: oidc  # local, ldap, oidc, saml
  oidc:
    issuer: "https://keycloak.example.com/realms/kubevirt"
    client_id: "kubevirt-shepherd"
    client_secret: "${OIDC_CLIENT_SECRET}"
```

### V2: JWT Session Revocation (Token Blacklist)

> Scope: Local JWT login sessions (`/api/v1/auth/login`), not VNC tokens.
> V1 remains stateless JWT without active revoke API.

#### Design Goals

- Support immediate logout and admin-forced token invalidation.
- Keep JWT validation deterministic across replicas.
- Keep storage PostgreSQL-only (no Redis hard dependency).

#### Data Model (PostgreSQL)

```sql
-- Active/revoked JWT sessions keyed by jti
CREATE TABLE auth_session_tokens (
    jti            varchar(64) PRIMARY KEY,
    user_id        varchar(64) NOT NULL,
    token_type     varchar(16) NOT NULL DEFAULT 'access',
    issued_at      timestamptz NOT NULL,
    expires_at     timestamptz NOT NULL,
    revoked_at     timestamptz,
    revoked_by     varchar(64),
    revoke_reason  varchar(255),
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_session_tokens_user_id ON auth_session_tokens(user_id);
CREATE INDEX idx_auth_session_tokens_expires_at ON auth_session_tokens(expires_at);
CREATE INDEX idx_auth_session_tokens_revoked_at ON auth_session_tokens(revoked_at);
```

#### API Surface (V2)

| Endpoint | Method | Purpose | Auth |
|----------|--------|---------|------|
| `/api/v1/auth/logout` | POST | Revoke current token (`jti`) | Authenticated |
| `/api/v1/auth/sessions` | GET | List current user's active sessions | Authenticated |
| `/api/v1/auth/sessions/{jti}` | DELETE | Revoke one of current user's sessions | Authenticated |
| `/api/v1/admin/auth/sessions` | GET | Query user sessions (filter/pagination) | PlatformAdmin |
| `/api/v1/admin/auth/sessions/{jti}/revoke` | POST | Force revoke token | PlatformAdmin |
| `/api/v1/admin/auth/users/{user_id}/revoke-all` | POST | Revoke all active sessions for user | PlatformAdmin |

#### Middleware Validation Order

1. Parse + verify signature/alg.
2. Validate standard claims (`iss`/`exp`/`nbf`/`iat`).
3. Read `jti`; reject if missing.
4. Check `auth_session_tokens` revocation state.
5. Inject user context and continue.

#### Operational Policies

- Default access token TTL: 24h (same as V1 baseline unless explicitly changed).
- Revocation check is mandatory for all protected APIs once V2 enabled.
- Cleanup job deletes expired rows older than retention window (e.g. 7-30 days).
- All revoke operations must write audit logs (`auth.token_revoke`, `auth.revoke_all`).

#### Compatibility and Rollout

- Feature flag: `security.auth_revocation_enabled` (default `false` in V1).
- Dual-run phase supported:
  - write session rows at login
  - keep revocation check optional until rollout completes
- Once enabled, all instances must share the same PostgreSQL revocation state.

---

## Trigger Conditions

- Enterprise compliance requires MFA/2FA
- Legacy enterprise IdP only supports SAML 2.0
- Concurrent session control or forced logout required

---

## References

- [ADR-0015: Governance Model V2 §22](../adr/ADR-0015-governance-model-v2.md) - OIDC/LDAP base implementation
- [OIDC Specification](https://openid.net/specs/openid-connect-core-1_0.html)
- [go-ldap](https://github.com/go-ldap/ldap)
