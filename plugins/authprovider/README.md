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

## Separate Repository Guidance

Private or external provider repositories should import only:

- `kv-shepherd.io/shepherd/pkg/authproviderplugin`

They must not import:

- `kv-shepherd.io/shepherd/internal/...`

The repository includes a blocking smoke module under:

- `tools/sdk-smoke/authproviderplugin-external`

That smoke test proves a separate Go module can compile and test against the
public SDK surface without any `internal/...` imports, and can also compile an
enterprise server entrypoint against `pkg/serverbootstrap`.

For the broader public/private repository collaboration model, see:

- `plugins/authprovider/PRIVATE_REPOSITORY_GUIDE.md`

## Separate Repository Author Workflow

For enterprise or private implementations that live outside this repository:

1. Create a separate Go module for the provider implementation.
2. Import only `kv-shepherd.io/shepherd/pkg/authproviderplugin`.
3. Implement `AdminAdapter` plus any optional runtime/directory capabilities
   needed by the provider.
4. Register the adapter with
   `authproviderplugin.RegisterAdminAdapter` or
   `authproviderplugin.MustRegisterAdminAdapter`.
5. Keep provider-specific mapping, transport, and deployment glue in the
   external repository; do not import Shepherd `internal/...`.
6. Use `tools/sdk-smoke/authproviderplugin-external` in this repository as the
   reference pattern for a compile-only external consumer.

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
