# RFC-0008: Extended Authentication Providers

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Enterprise SSO/LDAP/OAuth integration required

---

## Problem

V1.0 uses basic JWT authentication. Enterprise deployments may require:
- LDAP/Active Directory integration
- OIDC providers (Keycloak, Okta)
- Multi-factor authentication
- SAML 2.0 support

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

- Enterprise requires SSO integration
- LDAP-based user management needed
- Compliance requires MFA

---

## References

- [OIDC Specification](https://openid.net/specs/openid-connect-core-1_0.html)
- [go-ldap](https://github.com/go-ldap/ldap)
