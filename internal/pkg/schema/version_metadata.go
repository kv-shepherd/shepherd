package schema

import (
	"encoding/json"
	"fmt"
	"sort"

	semver "github.com/Masterminds/semver/v3"
)

type fieldSnapshot struct {
	Path        string
	Type        string
	Enum        string
	Description string
}

// VersionDiffSummary describes the current embedded schema baseline compared
// with its immediate previous embedded baseline for one entity.
type VersionDiffSummary struct {
	FromVersion      string
	ToVersion        string
	SchemaPathsAdded []string
	MaskPathsAdded   []string
	ChangedPaths     []string
}

// FieldIntroducedVersions returns schema-path-level version metadata for one entity.
//
// The returned map contains only paths that were introduced after the oldest
// embedded baseline. Fields already present in the oldest supported embedded
// version are intentionally omitted so the frontend can treat them as
// unversioned baseline fields.
func FieldIntroducedVersions(entityType string) (map[string]string, error) {
	versions, ok := AvailableVersions(entityType)
	if !ok || len(versions) == 0 {
		return nil, fmt.Errorf("unsupported entity type: %s", entityType)
	}

	firstSeen := make(map[string]string)
	for _, version := range versions {
		schemaBytes, ok := SchemaForVersion(entityType, version.Key)
		if !ok {
			return nil, fmt.Errorf("missing schema for entity %s version %s", entityType, version.Key)
		}
		paths, err := collectSchemaPaths(schemaBytes)
		if err != nil {
			return nil, fmt.Errorf("parse schema for entity %s version %s: %w", entityType, version.Key, err)
		}
		for path := range paths {
			if _, seen := firstSeen[path]; seen {
				continue
			}
			firstSeen[path] = version.KubeVirtVersion
		}
	}

	oldestBaseline := versions[0].KubeVirtVersion
	introduced := make(map[string]string)
	for path, version := range firstSeen {
		if version == oldestBaseline {
			continue
		}
		introduced[path] = version
	}
	return introduced, nil
}

// CurrentVersionDiffSummary compares the current embedded baseline with the
// previous embedded baseline for one entity. If there is no previous baseline,
// it returns nil, nil.
func CurrentVersionDiffSummary(entityType string) (*VersionDiffSummary, error) {
	versions, ok := AvailableVersions(entityType)
	if !ok || len(versions) == 0 {
		return nil, fmt.Errorf("unsupported entity type: %s", entityType)
	}
	currentVersionKey, ok := CurrentVersionKeyFor(entityType)
	if !ok {
		return nil, fmt.Errorf("missing current version for entity %s", entityType)
	}

	currentIdx := -1
	for i, version := range versions {
		if version.Key == currentVersionKey {
			currentIdx = i
			break
		}
	}
	if currentIdx == -1 {
		return nil, fmt.Errorf("current version key %s not present for entity %s", currentVersionKey, entityType)
	}
	if currentIdx == 0 {
		return nil, nil
	}

	fromVersion := versions[currentIdx-1]
	toVersion := versions[currentIdx]

	fromSchemaBytes, ok := SchemaForVersion(entityType, fromVersion.Key)
	if !ok {
		return nil, fmt.Errorf("missing schema for entity %s version %s", entityType, fromVersion.Key)
	}
	toSchemaBytes, ok := SchemaForVersion(entityType, toVersion.Key)
	if !ok {
		return nil, fmt.Errorf("missing schema for entity %s version %s", entityType, toVersion.Key)
	}
	fromSchema, err := flattenSchema(fromSchemaBytes)
	if err != nil {
		return nil, fmt.Errorf("parse schema for entity %s version %s: %w", entityType, fromVersion.Key, err)
	}
	toSchema, err := flattenSchema(toSchemaBytes)
	if err != nil {
		return nil, fmt.Errorf("parse schema for entity %s version %s: %w", entityType, toVersion.Key, err)
	}

	fromMaskBytes, ok := MaskForVersion(entityType, fromVersion.Key)
	if !ok {
		return nil, fmt.Errorf("missing mask for entity %s version %s", entityType, fromVersion.Key)
	}
	toMaskBytes, ok := MaskForVersion(entityType, toVersion.Key)
	if !ok {
		return nil, fmt.Errorf("missing mask for entity %s version %s", entityType, toVersion.Key)
	}
	fromMask, err := collectMaskPaths(fromMaskBytes)
	if err != nil {
		return nil, fmt.Errorf("parse mask for entity %s version %s: %w", entityType, fromVersion.Key, err)
	}
	toMask, err := collectMaskPaths(toMaskBytes)
	if err != nil {
		return nil, fmt.Errorf("parse mask for entity %s version %s: %w", entityType, toVersion.Key, err)
	}

	return &VersionDiffSummary{
		FromVersion:      fromVersion.KubeVirtVersion,
		ToVersion:        toVersion.KubeVirtVersion,
		SchemaPathsAdded: diffOnlySchemaPaths(toSchema, fromSchema),
		MaskPathsAdded:   diffOnlyPaths(toMask, fromMask),
		ChangedPaths:     diffChangedPaths(fromSchema, toSchema),
	}, nil
}

func sortEmbeddedVersions(versions []EmbeddedVersionInfo) {
	sort.SliceStable(versions, func(i, j int) bool {
		left, leftErr := semver.NewVersion(versions[i].KubeVirtVersion)
		right, rightErr := semver.NewVersion(versions[j].KubeVirtVersion)
		switch {
		case leftErr == nil && rightErr == nil:
			return left.LessThan(right)
		case leftErr == nil:
			return true
		case rightErr == nil:
			return false
		default:
			return versions[i].Key < versions[j].Key
		}
	})
}

func collectSchemaPaths(raw []byte) (map[string]struct{}, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	out := make(map[string]struct{})
	walkSchemaPaths(root, "", out)
	return out, nil
}

func flattenSchema(raw []byte) (map[string]fieldSnapshot, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	out := make(map[string]fieldSnapshot)
	walkFieldSnapshots(root, "", out)
	return out, nil
}

func walkFieldSnapshots(node any, currentPath string, out map[string]fieldSnapshot) {
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
			walkFieldSnapshots(properties[key], nextPath, out)
		}
	}

	if items, ok := objectNode["items"].(map[string]any); ok {
		nextPath := currentPath + "[]"
		if currentPath == "" {
			nextPath = "[]"
		}
		walkFieldSnapshots(items, nextPath, out)
	}

	if additional, ok := objectNode["additionalProperties"].(map[string]any); ok {
		nextPath := currentPath + ".*"
		if currentPath == "" {
			nextPath = "*"
		}
		walkFieldSnapshots(additional, nextPath, out)
	}
}

func walkSchemaPaths(node any, currentPath string, out map[string]struct{}) {
	objectNode, ok := node.(map[string]any)
	if !ok {
		return
	}

	if currentPath != "" {
		out[currentPath] = struct{}{}
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
			walkSchemaPaths(properties[key], nextPath, out)
		}
	}

	if items, ok := objectNode["items"].(map[string]any); ok {
		nextPath := currentPath + "[]"
		if currentPath == "" {
			nextPath = "[]"
		}
		walkSchemaPaths(items, nextPath, out)
	}

	if additional, ok := objectNode["additionalProperties"].(map[string]any); ok {
		nextPath := currentPath + ".*"
		if currentPath == "" {
			nextPath = "*"
		}
		walkSchemaPaths(additional, nextPath, out)
	}
}

func collectMaskPaths(raw []byte) (map[string]struct{}, error) {
	var mask struct {
		QuickFields []struct {
			Path string `json:"path"`
		} `json:"quick_fields"`
		AdvancedFields []struct {
			Path string `json:"path"`
		} `json:"advanced_fields"`
		ProfessionalFields []struct {
			Path string `json:"path"`
		} `json:"professional_fields"`
	}
	if err := json.Unmarshal(raw, &mask); err != nil {
		return nil, err
	}
	paths := make(map[string]struct{})
	for _, field := range append(append(mask.QuickFields, mask.AdvancedFields...), mask.ProfessionalFields...) {
		if field.Path == "" {
			continue
		}
		paths[field.Path] = struct{}{}
	}
	return paths, nil
}

func diffOnlySchemaPaths(left, right map[string]fieldSnapshot) []string {
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

func diffChangedPaths(left, right map[string]fieldSnapshot) []string {
	paths := make([]string, 0)
	for key, leftField := range left {
		rightField, ok := right[key]
		if !ok {
			continue
		}
		if leftField.Type != rightField.Type || leftField.Enum != rightField.Enum || leftField.Description != rightField.Description {
			paths = append(paths, key)
		}
	}
	sort.Strings(paths)
	return paths
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
		return joinStrings(parts, "|")
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
	return joinStrings(parts, ",")
}

func joinStrings(values []string, sep string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	}
	result := values[0]
	for _, value := range values[1:] {
		result += sep + value
	}
	return result
}
