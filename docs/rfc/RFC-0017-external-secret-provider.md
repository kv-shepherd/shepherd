# RFC-0017: External Secret Provider Integration

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Enterprise deployment requiring centralized secret management (Vault, AWS KMS, Azure Key Vault)

## Problem

V1 uses a simple precedence model for secrets:
- `env vars > DB-generated` (implemented)
- `KMS/secret manager > env vars > DB-generated` (documented as priority, not implemented)

The "KMS/secret manager" option in ADR-0025 is a **placeholder for future enterprise integration**. 
V1 does not implement external secret provider support, which may be required for:

1. **Compliance**: SOC 2, HIPAA, PCI-DSS require centralized key management
2. **Enterprise standards**: Organizations may mandate HashiCorp Vault or cloud KMS
3. **Key lifecycle**: External providers offer audit trails, automatic rotation, and access policies

## Proposed Solution

Introduce a `SecretProvider` interface abstraction:

```go
// internal/pkg/secrets/provider.go

type SecretProvider interface {
    // GetSecret retrieves a secret by name
    GetSecret(ctx context.Context, keyName string) ([]byte, error)
    
    // StoreSecret stores a secret (optional, not all providers support)
    StoreSecret(ctx context.Context, keyName string, value []byte) error
    
    // Type returns provider identifier
    Type() string  // "vault", "aws-kms", "azure-keyvault", "env", "db"
    
    // HealthCheck verifies provider connectivity
    HealthCheck(ctx context.Context) error
}
```

### Provider Implementations

| Provider | Implementation | Complexity | Dependencies |
|----------|----------------|------------|--------------|
| `DatabaseSecretProvider` | ✅ V1 | Low | PostgreSQL only |
| `EnvSecretProvider` | ✅ V1 | Low | None |
| `VaultSecretProvider` | ⏳ RFC-0017 | Medium | Vault SDK, network |
| `AWSKMSSecretProvider` | ⏳ RFC-0017 | Medium | AWS SDK, IAM |
| `AzureKeyVaultProvider` | ⏳ RFC-0017 | Medium | Azure SDK |

### Configuration

```yaml
# config.yaml
security:
  secret_provider: "vault"  # "db" (default), "env", "vault", "aws-kms"
  
  vault:
    address: "https://vault.company.com:8200"
    auth_method: "kubernetes"  # or "token", "approle"
    secret_path: "secret/data/shepherd"
    # Token/AppRole credentials from env vars (never in config file)
    
  aws_kms:
    region: "us-west-2"
    key_id: "alias/shepherd-encryption-key"
    # IAM role assumed via IRSA or instance profile
```

### Migration Path

When transitioning from DB-generated to external provider:

1. Configure new provider
2. Run `shepherd secrets migrate --from=db --to=vault`
3. Tool re-encrypts all sensitive fields using new provider
4. Verify migration success
5. Update config to use new provider
6. Remove old DB-stored secrets (optional, can keep as fallback)

## Trade-offs

### Pros
- Enterprise-grade secret management
- Centralized audit trail
- Automatic rotation capability (via provider)
- Compliance alignment (SOC 2, HIPAA)
- Multi-environment consistency

### Cons
- Additional infrastructure dependency
- Network latency for secret retrieval
- Provider-specific configuration complexity
- Potential availability issues if provider is down

### Mitigations
- Cache secrets in memory with configurable TTL
- Implement graceful degradation (fallback to DB if provider unavailable)
- Provide clear error messages for misconfiguration

## Implementation Notes

### Phase 1: Interface Definition
- Define `SecretProvider` interface
- Refactor existing code to use interface
- Implement `DatabaseSecretProvider` and `EnvSecretProvider`

### Phase 2: Vault Integration
- Implement `VaultSecretProvider`
- Support multiple auth methods (token, AppRole, Kubernetes)
- Add health check to cluster status

### Phase 3: Cloud KMS Integration
- Implement `AWSKMSSecretProvider`
- Implement `AzureKeyVaultProvider`
- Add provider selection to admin UI

### Estimated Effort

| Task | Hours |
|------|-------|
| Interface definition | 2-4 |
| Vault provider + tests | 8-12 |
| AWS KMS provider + tests | 8-12 |
| Migration tool | 4-6 |
| Documentation | 2-4 |
| **Total** | **~30-40h** |

## Relationship to Existing Decisions

| Document | Relationship |
|----------|--------------|
| **ADR-0025** | Defines bootstrap secrets; this RFC extends the provider abstraction |
| **RFC-0016** | Key rotation; this RFC provides infrastructure for external rotation |
| **01-contracts.md** | Notes `VaultProvider` as "Future" |

## References

- ADR-0025: Bootstrap Secrets Auto-Generation and Persistence
- RFC-0016: Secret Key Rotation
- HashiCorp Vault: https://www.vaultproject.io/docs
- AWS KMS: https://docs.aws.amazon.com/kms/
- Azure Key Vault: https://docs.microsoft.com/en-us/azure/key-vault/
- OWASP Secrets Management: https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html
