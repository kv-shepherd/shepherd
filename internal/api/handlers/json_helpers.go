package handlers

import (
	"encoding/json"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

// jsonParseSchema parses raw JSON bytes into a map[string]interface{}.
// Intended for parsing large embedded schema files once at startup.
func jsonParseSchema(data []byte, out *map[string]interface{}) error {
	return json.Unmarshal(data, out)
}

// jsonParseMask parses raw JSON bytes into a generated.SchemaMask struct.
func jsonParseMask(data []byte, out *generated.SchemaMask) error {
	return json.Unmarshal(data, out)
}
