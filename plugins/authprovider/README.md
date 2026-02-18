# Auth Provider Plugins

This directory contains auth-provider plugin examples and a template.

## Contract

Plugin packages implement the public contract in `pkg/authproviderplugin`:

- `type AdminAdapter interface`
- optional `AdminAdapterDescriber`
- registration via `authproviderplugin.RegisterAdminAdapter` / `MustRegisterAdminAdapter`

## Auto Registration

Runtime auto-registration is activated by importing:

- `plugins/authprovider/autoreg`

That package uses side-effect imports for plugin packages. Each plugin package
registers itself in `init()`.

## Add a New Plugin

1. Copy `plugins/authprovider/template` into a new package.
2. Implement `Type`, `ValidateConfig`, `TestConnection`, `SampleFields`.
3. (Optional) Implement `Describe` to expose metadata and JSON schema.
4. Register adapter in plugin `init()` using `MustRegisterAdminAdapter`.
5. Add a blank import in `plugins/authprovider/autoreg/autoreg.go`.
6. Verify `GET /api/v1/admin/auth-provider-types` includes your new type.
