package handlers

import (
	"encoding/json"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

// jsonParseSchema parses raw JSON bytes into a map[string]interface{}.
// Intended for parsing large embedded schema files once at startup.
func jsonParseSchema(data []byte) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// jsonParseMask parses raw JSON bytes into a generated.SchemaMask struct.
func jsonParseMask(data []byte, out *generated.SchemaMask) error {
	return json.Unmarshal(data, out)
}
