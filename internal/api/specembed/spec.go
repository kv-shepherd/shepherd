package specembed

import _ "embed"

// CanonicalSpec contains the canonical OpenAPI contract for runtime validation.
//
//go:embed openapi.yaml
var CanonicalSpec []byte
