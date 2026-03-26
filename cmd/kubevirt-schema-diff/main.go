package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kv-shepherd.io/shepherd/internal/pkg/schema"
)

type schemaMask struct {
	QuickFields        []maskField `json:"quick_fields"`
	AdvancedFields     []maskField `json:"advanced_fields"`
	ProfessionalFields []maskField `json:"professional_fields"`
}

type maskField struct {
	Path string `json:"path"`
}

type fieldSnapshot struct {
	Path        string
	Type        string
	Enum        string
	Description string
}

type schemaBundle struct {
	Label  string
	Schema []byte
	Mask   []byte
}

func main() {
	entityType := flag.String("entity", "instancesize", "schema entity type to compare")
	fromVersion := flag.String("from-version", "", "embedded version key for the source side")
	fromDir := flag.String("from-dir", "", "directory containing <entity>.schema.json and <entity>.mask.json for the source side")
	toVersion := flag.String("to-version", "", "embedded version key for the target side")
	toDir := flag.String("to-dir", "", "directory containing <entity>.schema.json and <entity>.mask.json for the target side")
	flag.Parse()

	from, err := loadBundle(*entityType, *fromVersion, *fromDir, "from")
	if err != nil {
		fatal(err)
	}
	to, err := loadBundle(*entityType, *toVersion, *toDir, "to")
	if err != nil {
		fatal(err)
	}

	fromSchema, err := flattenSchema(from.Schema)
	if err != nil {
		fatal(fmt.Errorf("parse source schema: %w", err))
	}
	toSchema, err := flattenSchema(to.Schema)
	if err != nil {
		fatal(fmt.Errorf("parse target schema: %w", err))
	}

	fromMask, err := collectMaskPaths(from.Mask)
	if err != nil {
		fatal(fmt.Errorf("parse source mask: %w", err))
	}
	toMask, err := collectMaskPaths(to.Mask)
	if err != nil {
		fatal(fmt.Errorf("parse target mask: %w", err))
	}

	fmt.Printf("Schema diff for %s\n", *entityType)
	fmt.Printf("from: %s\n", from.Label)
	fmt.Printf("to:   %s\n\n", to.Label)

	printPathSetDiff("Schema paths added", diffOnlyKeys(toSchema, fromSchema))
	printPathSetDiff("Schema paths removed", diffOnlyKeys(fromSchema, toSchema))
	printChangedFields("Schema field changes", diffChangedFields(fromSchema, toSchema))
	printPathSetDiff("Mask paths added", diffOnlyPaths(toMask, fromMask))
	printPathSetDiff("Mask paths removed", diffOnlyPaths(fromMask, toMask))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func loadBundle(entityType, versionKey, dir, side string) (*schemaBundle, error) {
	if strings.TrimSpace(versionKey) != "" && strings.TrimSpace(dir) != "" {
		return nil, fmt.Errorf("%s: choose either --%s-version or --%s-dir", side, side, side)
	}
	if strings.TrimSpace(versionKey) == "" && strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("%s: one of --%s-version or --%s-dir is required", side, side, side)
	}
	if strings.TrimSpace(versionKey) != "" {
		schemaBytes, ok := schema.SchemaForVersion(entityType, versionKey)
		if !ok {
			return nil, fmt.Errorf("%s: unknown embedded version %q for entity %q", side, versionKey, entityType)
		}
		maskBytes, ok := schema.MaskForVersion(entityType, versionKey)
		if !ok {
			return nil, fmt.Errorf("%s: missing embedded mask for version %q and entity %q", side, versionKey, entityType)
		}
		return &schemaBundle{
			Label:  "embedded:" + versionKey,
			Schema: schemaBytes,
			Mask:   maskBytes,
		}, nil
	}

	schemaPath := filepath.Join(dir, entityType+".schema.json")
	maskPath := filepath.Join(dir, entityType+".mask.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("%s: read %s: %w", side, schemaPath, err)
	}
	maskBytes, err := os.ReadFile(maskPath)
	if err != nil {
		return nil, fmt.Errorf("%s: read %s: %w", side, maskPath, err)
	}
	return &schemaBundle{
		Label:  "dir:" + dir,
		Schema: schemaBytes,
		Mask:   maskBytes,
	}, nil
}

func flattenSchema(raw []byte) (map[string]fieldSnapshot, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	out := make(map[string]fieldSnapshot)
	walkSchema(root, "", out)
	return out, nil
}

func walkSchema(node any, currentPath string, out map[string]fieldSnapshot) {
	objectNode, ok := node.(map[string]any)
	if !ok {
		return
	}

	if currentPath != "" {
		out[currentPath] = fieldSnapshot{
			Path:        currentPath,
			Type:        scalarString(objectNode["type"]),
			Enum:        joinEnum(objectNode["enum"]),
			Description: scalarString(objectNode["description"]),
		}
	}

	if properties, ok := objectNode["properties"].(map[string]any); ok {
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			nextPath := key
			if currentPath != "" {
				nextPath = currentPath + "." + key
			}
			walkSchema(properties[key], nextPath, out)
		}
	}

	if items, ok := objectNode["items"].(map[string]any); ok {
		nextPath := currentPath + "[]"
		if currentPath == "" {
			nextPath = "[]"
		}
		walkSchema(items, nextPath, out)
	}

	if additional, ok := objectNode["additionalProperties"].(map[string]any); ok {
		nextPath := currentPath + ".*"
		if currentPath == "" {
			nextPath = "*"
		}
		walkSchema(additional, nextPath, out)
	}
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, scalarString(item))
		}
		return strings.Join(parts, "|")
	default:
		return ""
	}
}

func joinEnum(value any) string {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, scalarString(item))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func collectMaskPaths(raw []byte) (map[string]struct{}, error) {
	var mask schemaMask
	if err := json.Unmarshal(raw, &mask); err != nil {
		return nil, err
	}
	paths := make(map[string]struct{})
	for _, field := range append(append(mask.QuickFields, mask.AdvancedFields...), mask.ProfessionalFields...) {
		if strings.TrimSpace(field.Path) == "" {
			continue
		}
		paths[strings.TrimSpace(field.Path)] = struct{}{}
	}
	return paths, nil
}

func diffOnlyKeys(left, right map[string]fieldSnapshot) []string {
	paths := make([]string, 0)
	for key := range left {
		if _, ok := right[key]; !ok {
			paths = append(paths, key)
		}
	}
	sort.Strings(paths)
	return paths
}

func diffOnlyPaths(left, right map[string]struct{}) []string {
	paths := make([]string, 0)
	for key := range left {
		if _, ok := right[key]; !ok {
			paths = append(paths, key)
		}
	}
	sort.Strings(paths)
	return paths
}

func diffChangedFields(left, right map[string]fieldSnapshot) []string {
	paths := make([]string, 0)
	for key, leftField := range left {
		rightField, ok := right[key]
		if !ok {
			continue
		}
		if leftField.Type != rightField.Type || leftField.Enum != rightField.Enum || leftField.Description != rightField.Description {
			paths = append(paths, fmt.Sprintf(
				"%s\n  type: %q -> %q\n  enum: %q -> %q\n  description: %q -> %q",
				key,
				leftField.Type,
				rightField.Type,
				leftField.Enum,
				rightField.Enum,
				leftField.Description,
				rightField.Description,
			))
		}
	}
	sort.Strings(paths)
	return paths
}

func printPathSetDiff(title string, values []string) {
	fmt.Printf("%s: %d\n", title, len(values))
	for _, value := range values {
		fmt.Printf("- %s\n", value)
	}
	fmt.Println()
}

func printChangedFields(title string, values []string) {
	fmt.Printf("%s: %d\n", title, len(values))
	for _, value := range values {
		fmt.Printf("- %s\n", value)
	}
	fmt.Println()
}
