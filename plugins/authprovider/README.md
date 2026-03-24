# Auth Provider Plugins

This directory contains auth-provider plugin examples and a template.

## Contract

Plugin packages implement the public contract in `pkg/authproviderplugin`:

- `type AdminAdapter interface`
- optional `AdminAdapterDescriber`
- optional `RuntimeCapability`
- optional `CredentialRuntimeCapability`
- optional `RuntimeDescriber`
- optional `DirectorySyncCapability`
- optional `ScheduledDirectoryEnrichmentCapability`
- registration via `authproviderplugin.RegisterAdminAdapter` / `MustRegisterAdminAdapter`

Current model:

- registration is performed through the public admin adapter registry
- runtime login support is discovered by optional `RuntimeCapability`
- direct credential login support is discovered by optional `CredentialRuntimeCapability`
- directory sync/enrichment support is discovered by optional `DirectorySyncCapability`
- provider-owned scheduled enrichment plans are discovered by optional `ScheduledDirectoryEnrichmentCapability`

Plugin authors should only depend on `pkg/authproviderplugin`, not
`internal/provider`.

## Auto Registration

Runtime auto-registration is activated by importing:

- `plugins/authprovider/autoreg`

That package uses side-effect imports for plugin packages. Each plugin package
registers itself in `init()`.

## Add a New Plugin

1. Copy `plugins/authprovider/template` into a new package.
2. Implement `Type`, `ValidateConfig`, `TestConnection`, `SampleFields`.
3. (Optional) Implement `Describe` to expose metadata and JSON schema.
4. (Optional) Implement `RuntimeCapability`, `CredentialRuntimeCapability`, and/or `RuntimeDescriber` for runtime login.
5. (Optional) Implement `DirectorySyncCapability` for directory sync or enrichment.
6. (Optional) Implement `ScheduledDirectoryEnrichmentCapability` when the provider can publish a scheduler plan.
7. Register adapter in plugin `init()` using `MustRegisterAdminAdapter`.
8. Add a blank import in `plugins/authprovider/autoreg/autoreg.go`.
9. Verify `GET /api/v1/admin/auth-provider-types` includes your new type.
