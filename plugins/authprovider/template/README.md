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
