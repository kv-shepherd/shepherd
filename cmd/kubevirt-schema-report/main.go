package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kv-shepherd.io/shepherd/internal/pkg/schema"
)

const (
	adminLocaleENPath = "web/src/i18n/locales/en/admin.json"
	adminLocaleZHPath = "web/src/i18n/locales/zh-CN/admin.json"
)

type schemaMask struct {
	QuickFields        []maskField `json:"quick_fields"`
	AdvancedFields     []maskField `json:"advanced_fields"`
	ProfessionalFields []maskField `json:"professional_fields"`
}

type maskField struct {
	Group          string
	Path           string `json:"path"`
	DisplayNameKey string `json:"display_name_key"`
	HelpKey        string `json:"help_key"`
	PlaceholderKey string `json:"placeholder_key"`
}

type fieldSnapshot struct {
	Path        string
	Type        string
	Enum        string
	Description string
}

type localeGap struct {
	Path      string
	Key       string
	KeyKind   string
	MissingEN bool
	MissingZH bool
}

type candidateField struct {
	Path               string
	IntroducedIn       string
	Type               string
	Enum               string
	Description        string
	SuggestedDisplay   string
	SuggestedHelp      string
	SuggestedPlacehold string
}

type changedMaskedField struct {
	Path           string
	Group          string
	DisplayNameKey string
	HelpKey        string
	PlaceholderKey string
	Type           string
	Enum           string
	Description    string
}

type upgradeReport struct {
	Entity             string
	FromVersion        string
	ToVersion          string
	SchemaPathsAdded   []candidateField
	ChangedMaskedPaths []changedMaskedField
	MissingLocaleKeys  []localeGap
}

func main() {
	entityType := flag.String("entity", "instancesize", "schema entity type to audit")
	flag.Parse()

	report, err := buildReport(strings.TrimSpace(*entityType))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	printReport(report)
}

func buildReport(entityType string) (*upgradeReport, error) {
	diff, err := schema.CurrentVersionDiffSummary(entityType)
	if err != nil {
		return nil, err
	}
	if diff == nil {
		return nil, fmt.Errorf("entity %q has no previous baseline to diff against", entityType)
	}

	currentSchemaBytes, ok := schema.SchemaFor(entityType)
	if !ok {
		return nil, fmt.Errorf("missing current schema for entity %q", entityType)
	}
	currentMaskBytes, ok := schema.MaskFor(entityType)
	if !ok {
		return nil, fmt.Errorf("missing current mask for entity %q", entityType)
	}

	currentSchema, err := flattenSchema(currentSchemaBytes)
	if err != nil {
		return nil, fmt.Errorf("parse current schema: %w", err)
	}
	currentMaskFields, err := collectMaskFields(currentMaskBytes)
	if err != nil {
		return nil, fmt.Errorf("parse current mask: %w", err)
	}
	maskByPath := make(map[string]maskField, len(currentMaskFields))
	for _, field := range currentMaskFields {
		maskByPath[field.Path] = field
	}

	introduced, err := schema.FieldIntroducedVersions(entityType)
	if err != nil {
		return nil, fmt.Errorf("build introduced_in metadata: %w", err)
	}

	addedCandidates := make([]candidateField, 0)
	for _, path := range diff.SchemaPathsAdded {
		if _, exposed := maskByPath[path]; exposed {
			continue
		}
		info := currentSchema[path]
		addedCandidates = append(addedCandidates, candidateField{
			Path:               path,
			IntroducedIn:       introduced[path],
			Type:               info.Type,
			Enum:               info.Enum,
			Description:        info.Description,
			SuggestedDisplay:   buildSuggestedLocaleKey(path, ""),
			SuggestedHelp:      buildSuggestedLocaleKey(path, "_help"),
			SuggestedPlacehold: buildSuggestedLocaleKey(path, "_placeholder"),
		})
	}

	changedMasked := make([]changedMaskedField, 0)
	for _, path := range diff.ChangedPaths {
		field, exposed := maskByPath[path]
		if !exposed {
			continue
		}
		info := currentSchema[path]
		changedMasked = append(changedMasked, changedMaskedField{
			Path:           path,
			Group:          field.Group,
			DisplayNameKey: field.DisplayNameKey,
			HelpKey:        field.HelpKey,
			PlaceholderKey: field.PlaceholderKey,
			Type:           info.Type,
			Enum:           info.Enum,
			Description:    info.Description,
		})
	}

	enLocale, err := readFlatLocale(adminLocaleENPath)
	if err != nil {
		return nil, err
	}
	zhLocale, err := readFlatLocale(adminLocaleZHPath)
	if err != nil {
		return nil, err
	}

	missingLocaleKeys := auditMaskLocaleKeys(currentMaskFields, enLocale, zhLocale)

	return &upgradeReport{
		Entity:             entityType,
		FromVersion:        diff.FromVersion,
		ToVersion:          diff.ToVersion,
		SchemaPathsAdded:   addedCandidates,
		ChangedMaskedPaths: changedMasked,
		MissingLocaleKeys:  missingLocaleKeys,
	}, nil
}

func printReport(report *upgradeReport) {
	printTo(os.Stdout, report)
}

func printTo(w io.Writer, report *upgradeReport) {
	fmt.Fprintf(w, "KubeVirt schema upgrade report for %s\n", report.Entity)
	fmt.Fprintf(w, "baseline: v%s -> v%s\n\n", report.FromVersion, report.ToVersion)

	fmt.Fprintf(w, "Schema additions not yet exposed in mask (%d)\n", len(report.SchemaPathsAdded))
	for i := range report.SchemaPathsAdded {
		item := &report.SchemaPathsAdded[i]
		fmt.Fprintf(w, "- %s", item.Path)
		if item.IntroducedIn != "" {
			fmt.Fprintf(w, "  [v%s+]", item.IntroducedIn)
		}
		fmt.Fprintln(w)
		if item.Type != "" {
			fmt.Fprintf(w, "  type: %s\n", item.Type)
		}
		if item.Enum != "" {
			fmt.Fprintf(w, "  enum: %s\n", item.Enum)
		}
		if item.Description != "" {
			fmt.Fprintf(w, "  description: %s\n", item.Description)
		}
		fmt.Fprintf(w, "  suggested display_name_key: %s\n", item.SuggestedDisplay)
		fmt.Fprintf(w, "  suggested help_key: %s\n", item.SuggestedHelp)
		fmt.Fprintf(w, "  suggested placeholder_key: %s\n", item.SuggestedPlacehold)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Exposed mask fields changed since previous baseline (%d)\n", len(report.ChangedMaskedPaths))
	for i := range report.ChangedMaskedPaths {
		item := &report.ChangedMaskedPaths[i]
		fmt.Fprintf(w, "- %s [%s]\n", item.Path, item.Group)
		if item.Type != "" {
			fmt.Fprintf(w, "  type: %s\n", item.Type)
		}
		if item.Enum != "" {
			fmt.Fprintf(w, "  enum: %s\n", item.Enum)
		}
		if item.Description != "" {
			fmt.Fprintf(w, "  description: %s\n", item.Description)
		}
		if item.DisplayNameKey != "" {
			fmt.Fprintf(w, "  display_name_key: %s\n", item.DisplayNameKey)
		}
		if item.HelpKey != "" {
			fmt.Fprintf(w, "  help_key: %s\n", item.HelpKey)
		}
		if item.PlaceholderKey != "" {
			fmt.Fprintf(w, "  placeholder_key: %s\n", item.PlaceholderKey)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Missing admin locale keys referenced by current mask (%d)\n", len(report.MissingLocaleKeys))
	for _, item := range report.MissingLocaleKeys {
		status := make([]string, 0, 2)
		if item.MissingEN {
			status = append(status, "en")
		}
		if item.MissingZH {
			status = append(status, "zh-CN")
		}
		fmt.Fprintf(w, "- %s (%s) → missing %s\n", item.Key, item.KeyKind, strings.Join(status, ", "))
		fmt.Fprintf(w, "  path: %s\n", item.Path)
	}
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

func collectMaskFields(raw []byte) ([]maskField, error) {
	var mask schemaMask
	if err := json.Unmarshal(raw, &mask); err != nil {
		return nil, err
	}
	out := make([]maskField, 0, len(mask.QuickFields)+len(mask.AdvancedFields)+len(mask.ProfessionalFields))
	for _, field := range mask.QuickFields {
		field.Group = "quick_fields"
		out = append(out, field)
	}
	for _, field := range mask.AdvancedFields {
		field.Group = "advanced_fields"
		out = append(out, field)
	}
	for _, field := range mask.ProfessionalFields {
		field.Group = "professional_fields"
		out = append(out, field)
	}
	return out, nil
}

func readFlatLocale(filePath string) (map[string]string, error) {
	resolvedPath, err := resolveRepoPath(filePath)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("read locale %s: %w", resolvedPath, err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse locale %s: %w", resolvedPath, err)
	}
	flat := make(map[string]string)
	flattenLocaleMap("", root, flat)
	return flat, nil
}

func resolveRepoPath(path string) (string, error) {
	candidates := []string{
		path,
		filepath.Join("..", "..", path),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("resolve repo path %s: file not found", path)
}

func flattenLocaleMap(prefix string, value any, out map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			flattenLocaleMap(next, nested, out)
		}
	case string:
		if prefix != "" {
			out[prefix] = typed
		}
	}
}

func auditMaskLocaleKeys(fields []maskField, enLocale, zhLocale map[string]string) []localeGap {
	gaps := make([]localeGap, 0)
	for _, field := range fields {
		gaps = appendLocaleGap(gaps, field.Path, field.DisplayNameKey, "display_name_key", enLocale, zhLocale)
		gaps = appendLocaleGap(gaps, field.Path, field.HelpKey, "help_key", enLocale, zhLocale)
		gaps = appendLocaleGap(gaps, field.Path, field.PlaceholderKey, "placeholder_key", enLocale, zhLocale)
	}
	sort.SliceStable(gaps, func(i, j int) bool {
		if gaps[i].Path == gaps[j].Path {
			return gaps[i].Key < gaps[j].Key
		}
		return gaps[i].Path < gaps[j].Path
	})
	return gaps
}

func appendLocaleGap(gaps []localeGap, path, key, keyKind string, enLocale, zhLocale map[string]string) []localeGap {
	key = strings.TrimSpace(key)
	if key == "" {
		return gaps
	}
	_, hasEN := enLocale[key]
	_, hasZH := zhLocale[key]
	if hasEN && hasZH {
		return gaps
	}
	return append(gaps, localeGap{
		Path:      path,
		Key:       key,
		KeyKind:   keyKind,
		MissingEN: !hasEN,
		MissingZH: !hasZH,
	})
}

func buildSuggestedLocaleKey(path, suffix string) string {
	normalized := strings.TrimSpace(path)
	normalized = strings.ReplaceAll(normalized, "[]", "_items")
	normalized = strings.ReplaceAll(normalized, "*", "all")
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range normalized {
		switch {
		case r >= 'A' && r <= 'Z':
			if builder.Len() > 0 && !lastUnderscore {
				builder.WriteByte('_')
			}
			builder.WriteRune(r + ('a' - 'A'))
			lastUnderscore = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore && builder.Len() > 0 {
				builder.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	value := strings.Trim(builder.String(), "_")
	return "instanceSizes.mask." + value + suffix
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
