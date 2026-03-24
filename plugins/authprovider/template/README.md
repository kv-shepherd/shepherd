# Auth Provider Plugin Template

Use `template.Adapter` as a starting point for custom provider plugins.

Example:

```go
package myprovider

import (
    "kv-shepherd.io/shepherd/pkg/authproviderplugin"
    templ "kv-shepherd.io/shepherd/plugins/authprovider/template"
)

var adapter = templ.New("my-provider", "My Provider")

func init() {
    authproviderplugin.MustRegisterAdminAdapter(adapter)
}
```

Then add a blank import in `plugins/authprovider/autoreg/autoreg.go`.

You may optionally extend the template adapter by implementing:

- `authproviderplugin.RuntimeCapability`
- `authproviderplugin.CredentialRuntimeCapability`
- `authproviderplugin.RuntimeDescriber`
- `authproviderplugin.DirectorySyncCapability`
- `authproviderplugin.ScheduledDirectoryEnrichmentCapability`

The adapter should continue to register through
`authproviderplugin.MustRegisterAdminAdapter(...)`.
