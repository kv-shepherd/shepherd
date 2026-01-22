# RFC-0008: Extended Authentication Providers

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Enterprise requires MFA or SAML 2.0 support

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
- Advanced session management (concurrent session limits, forced logout)

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
