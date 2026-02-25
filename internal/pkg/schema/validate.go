package schema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateMaskPaths verifies that every path in mask.quick_fields and
// mask.advanced_fields resolves within the provided JSON Schema.
//
// This implements Stage 1 requirement: "Invalid mask paths must fail
// validation before deployment." (master-flow.md:186)
//
// The check is path-navigation based: for a path like
// "spec.template.spec.domain.cpu.cores", this function walks the nested
// "properties" / "items.properties" hierarchy of the JSON Schema and
// confirms the final key exists. Array-typed nodes (type: "array") may be
// traversed through their "items" sub-schema.
//
// Returns a non-nil error listing ALL invalid paths, not just the first,
// so that operator can fix all issues in one deployment cycle.
func ValidateMaskPaths(schemaJSON, maskJSON []byte) error {
	var rawSchema map[string]interface{}
	if err := json.Unmarshal(schemaJSON, &rawSchema); err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}

	var mask struct {
		QuickFields []struct {
			Path string `json:"path"`
		} `json:"quick_fields"`
		AdvancedFields []struct {
			Path string `json:"path"`
		} `json:"advanced_fields"`
	}
	if err := json.Unmarshal(maskJSON, &mask); err != nil {
		return fmt.Errorf("parse mask: %w", err)
	}

	var invalid []string
	check := func(section, path string) {
		if path == "" {
			invalid = append(invalid, fmt.Sprintf("%s[empty path]", section))
			return
		}
		if err := resolveSchemaPath(rawSchema, path); err != nil {
			invalid = append(invalid, fmt.Sprintf("%s path %q: %v", section, path, err))
		}
	}

	for _, f := range mask.QuickFields {
		check("quick_fields", f.Path)
	}
	for _, f := range mask.AdvancedFields {
		check("advanced_fields", f.Path)
	}

	if len(invalid) > 0 {
		return fmt.Errorf("mask has %d invalid path(s):\n  - %s",
			len(invalid), strings.Join(invalid, "\n  - "))
	}
	return nil
}

// resolveSchemaPath navigates a JSON Schema object following dot-notation path.
// Traversal rules (JSON Schema Draft-07):
//   - At each segment, look for the key in current node's "properties" map.
//   - If the current node has type "array", descend into "items" before
//     trying "properties" again (covers array-of-objects paths).
//   - Leaf segments just need to exist; their type is validated by the
//     schema itself (not this validator).
func resolveSchemaPath(schema map[string]interface{}, path string) error {
	segments := strings.Split(path, ".")
	cur := schema

	for i, seg := range segments {
		// If current node is array-typed, descend into items before trying properties.
		if t, ok := cur["type"]; ok && t == "array" {
			items, hasItems := cur["items"]
			if !hasItems {
				return fmt.Errorf("segment[%d] %q: array node has no 'items'", i, seg)
			}
			itemsMap, ok := items.(map[string]interface{})
			if !ok {
				return fmt.Errorf("segment[%d] %q: 'items' is not an object", i, seg)
			}
			cur = itemsMap
		}

		props, ok := cur["properties"]
		if !ok {
			return fmt.Errorf("segment[%d] %q: node has no 'properties'", i, seg)
		}
		propsMap, ok := props.(map[string]interface{})
		if !ok {
			return fmt.Errorf("segment[%d] %q: 'properties' is not an object", i, seg)
		}
		next, ok := propsMap[seg]
		if !ok {
			available := make([]string, 0, len(propsMap))
			for k := range propsMap {
				available = append(available, k)
			}
			return fmt.Errorf("segment[%d] %q: not found in properties (available: %v)",
				i, seg, available)
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			// Leaf node with non-object definition (e.g. primitive type): valid if last seg.
			if i == len(segments)-1 {
				return nil
			}
			return fmt.Errorf("segment[%d] %q: not an object, cannot traverse further", i, seg)
		}
		cur = nextMap
	}
	return nil
}
