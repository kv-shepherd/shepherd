# ADR-0026 Design Notes: Auth Provider Naming Unification

> **Status**: Pending ADR-0026 acceptance

## Summary

- Canonical table name is `auth_providers`.
- Replace references to `idp_configs` in design docs and schemas.
- Adapter/plugin layer maps external providers (OIDC/LDAP/SSO/WeCom/Feishu/DingTalk) to standard output fields.

## Design Impact

- Schema: `auth_providers` includes issuer, client_id, client_secret_encrypted, claims_mapping, defaults.
- API/UI labels use Auth Provider terminology.
- Migration plan required if `idp_configs` ever implemented.

## Standard Provider Output (Contract)

Adapters normalize to:

| Field | Type | Description |
|-------|------|-------------|
| `provider_id` | string | `auth_providers.id` |
| `auth_type` | string | `oidc` / `ldap` / `sso` / `wecom` / `feishu` / `dingtalk` |
| `external_id` | string | Stable subject identifier |
| `email` | string | May be empty |
| `display_name` | string | Human-readable name |
| `groups` | string[] | Normalized group list |
| `raw_claims` | json | Optional raw attributes |

## References

- ADR-0026 (proposed)
- ADR-0015 §22.6 (IdP config schema, amended by ADR-0026)
