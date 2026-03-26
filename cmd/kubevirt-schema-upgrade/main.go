package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const manifestPath = "internal/pkg/schema/manifest.json"

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

type manifest struct {
	Entities map[string]manifestEntity `json:"entities"`
}

type manifestEntity struct {
	CurrentVersion string                     `json:"current_version"`
	Versions       map[string]manifestVersion `json:"versions"`
}

type manifestVersion struct {
	KubeVirtVersion string `json:"kubevirt_version"`
	SchemaPath      string `json:"schema_path"`
	MaskPath        string `json:"mask_path"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: kubevirt-schema-upgrade <kubevirt-version>  e.g. 1.8.0")
		os.Exit(1)
	}
	version := os.Args[1]
	if !versionRe.MatchString(version) {
		fatalf("invalid kubevirt version %q: expected semantic version like 1.8.0", version)
	}
	versionKey := "kubevirt-v" + version
	schemaDir := filepath.Join("internal", "pkg", "schema", "versions", versionKey)

	m, err := readManifest()
	if err != nil {
		fatalf("read manifest: %v", err)
	}

	var currentVersionKey string
	for _, entity := range m.Entities {
		currentVersionKey = entity.CurrentVersion
		break
	}
	if currentVersionKey == "" {
		fatalf("no current_version in manifest")
	}

	if versionKey == currentVersionKey {
		fmt.Printf("Schema is already at %s — nothing to do.\n", versionKey)
		os.Exit(0)
	}

	swaggerURL := fmt.Sprintf(
		"https://github.com/kubevirt/kubevirt/releases/download/v%s/swagger.json", version)
	fmt.Printf("📥 Downloading KubeVirt v%s swagger.json ...\n", version)

	swaggerData, err := httpGet(swaggerURL)
	if err != nil {
		fatalf("download swagger.json: %v\n  Check: https://github.com/kubevirt/kubevirt/releases/tag/v%s", err, version)
	}
	fmt.Printf("✅ Downloaded %d bytes\n", len(swaggerData))

	fmt.Println("🔧 Extracting VirtualMachineSpec ...")
	schema, err := extractVMSpec(swaggerData, version)
	if err != nil {
		fatalf("extract schema: %v", err)
	}

	// #nosec G703 -- version is semver-validated above and schemaDir stays under the repository tree.
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		fatalf("mkdir %s: %v", schemaDir, err)
	}

	newSchemaPath := filepath.Join(schemaDir, "instancesize.schema.json")
	schemaJSON, _ := json.MarshalIndent(schema, "", "  ")
	// #nosec G306,G703 -- repository JSON artifacts are intentionally checked in world-readable under a semver-validated path.
	if err := os.WriteFile(newSchemaPath, schemaJSON, 0o644); err != nil {
		fatalf("write schema: %v", err)
	}
	fmt.Printf("✅ Schema written: %s (%d bytes)\n", newSchemaPath, len(schemaJSON))

	currentMaskPath := filepath.Join("internal", "pkg", "schema", "versions",
		currentVersionKey, "instancesize.mask.json")
	newMaskPath := filepath.Join(schemaDir, "instancesize.mask.json")

	if maskData, err := os.ReadFile(currentMaskPath); err == nil {
		// #nosec G306,G703 -- repository JSON artifacts are intentionally checked in world-readable under a semver-validated path.
		if err := os.WriteFile(newMaskPath, maskData, 0o644); err != nil {
			fmt.Printf("⚠️  Failed to copy mask: %v\n", err)
		} else {
			fmt.Printf("📋 Copied mask from %s as baseline\n", currentVersionKey)
		}
	} else {
		fmt.Printf("⚠️  No existing mask found at %s\n", currentMaskPath)
	}

	for entityName, entity := range m.Entities {
		entity.CurrentVersion = versionKey
		if entity.Versions == nil {
			entity.Versions = make(map[string]manifestVersion)
		}
		entity.Versions[versionKey] = manifestVersion{
			KubeVirtVersion: version,
			SchemaPath:      fmt.Sprintf("versions/%s/instancesize.schema.json", versionKey),
			MaskPath:        fmt.Sprintf("versions/%s/instancesize.mask.json", versionKey),
		}
		m.Entities[entityName] = entity
	}
	if err := writeManifest(m); err != nil {
		fatalf("update manifest: %v", err)
	}
	fmt.Printf("✅ manifest.json updated: current_version → %s\n", versionKey)

	currentSchemaPath := filepath.Join("internal", "pkg", "schema", "versions",
		currentVersionKey, "instancesize.schema.json")
	printDiffSummary(currentSchemaPath, newSchemaPath, currentVersionKey, versionKey)

	fmt.Println()
	fmt.Println("📝 Next steps:")
	fmt.Println("  1. Review the diff above")
	fmt.Println("  2. Check if new fields need to be added to instancesize.mask.json")
	fmt.Printf("  3. Update embed_test.go version assertions → %q\n", versionKey)
	fmt.Printf("  4. Update go.mod: kubevirt.io/api + kubevirt.io/client-go to v%s\n", version)
	fmt.Println("  5. Run 'make test' to verify")
}

func extractVMSpec(swaggerData []byte, version string) (map[string]any, error) {
	var swagger struct {
		Definitions map[string]any `json:"definitions"`
	}
	if err := json.Unmarshal(swaggerData, &swagger); err != nil {
		return nil, fmt.Errorf("parse swagger: %w", err)
	}
	if swagger.Definitions == nil {
		return nil, fmt.Errorf("no definitions in swagger.json")
	}

	var vmSpecKey string
	for key := range swagger.Definitions {
		if strings.HasSuffix(key, ".VirtualMachineSpec") || key == "v1.VirtualMachineSpec" {
			vmSpecKey = key
			break
		}
	}
	if vmSpecKey == "" {
		var candidates []string
		for key := range swagger.Definitions {
			if strings.Contains(key, "VirtualMachine") {
				candidates = append(candidates, key)
			}
		}
		return nil, fmt.Errorf("VirtualMachineSpec not found; candidates: %v", candidates)
	}
	fmt.Printf("   Found definition: %s\n", vmSpecKey)

	vmSpec := swagger.Definitions[vmSpecKey]
	resolved := resolveRefs(vmSpec, swagger.Definitions, 0)
	resolvedMap, ok := resolved.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("resolved spec is not an object")
	}
	templateProp := extractNested(resolvedMap, "properties", "template")

	schema := map[string]any{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"$id":         fmt.Sprintf("kv-shepherd:instancesize:kubevirt-v%s", version),
		"type":        "object",
		"title":       "KubeVirt VirtualMachine Spec Overrides",
		"description": fmt.Sprintf("Official KubeVirt v%s VirtualMachineSpec schema (source: kubevirt/api, v%s). Generated from CRD OpenAPI swagger.json for use with spec_overrides (ADR-0023).", version, version),
		"properties": map[string]any{
			"spec": map[string]any{
				"type":        "object",
				"description": "KubeVirt VirtualMachineSpec (spec_overrides target root).",
				"properties": map[string]any{
					"template": templateProp,
				},
			},
		},
	}
	return schema, nil
}

func resolveRefs(obj any, defs map[string]any, depth int) any {
	if depth > 20 {
		return obj
	}
	switch v := obj.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok {
			parts := strings.Split(ref, "/")
			refKey := parts[len(parts)-1]
			if resolved, ok := defs[refKey]; ok {
				return resolveRefs(deepCopy(resolved), defs, depth+1)
			}
			return v
		}
		result := make(map[string]any, len(v))
		for key, val := range v {
			result[key] = resolveRefs(val, defs, depth)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = resolveRefs(val, defs, depth)
		}
		return result
	default:
		return obj
	}
}

func deepCopy(v any) any {
	data, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(data, &out)
	return out
}

func extractNested(obj map[string]any, keys ...string) any {
	var current any = obj
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[key]
	}
	return current
}

func printDiffSummary(oldPath, newPath, oldKey, newKey string) {
	fmt.Println()
	fmt.Printf("============================================================\n")
	fmt.Printf("📊 Schema Diff Summary (%s → %s)\n", oldKey, newKey)
	fmt.Printf("============================================================\n")

	oldPaths := extractPropertyPaths(oldPath)
	newPaths := extractPropertyPaths(newPath)

	added := difference(newPaths, oldPaths)
	removed := difference(oldPaths, newPaths)

	if len(added) > 0 {
		fmt.Printf("\n🟢 New fields (%d):\n", len(added))
		sort.Strings(added)
		for i, p := range added {
			if i >= 30 {
				fmt.Printf("  ... and %d more\n", len(added)-30)
				break
			}
			fmt.Printf("  + %s\n", p)
		}
	}

	if len(removed) > 0 {
		fmt.Printf("\n🔴 Removed fields (%d):\n", len(removed))
		sort.Strings(removed)
		for i, p := range removed {
			if i >= 30 {
				fmt.Printf("  ... and %d more\n", len(removed)-30)
				break
			}
			fmt.Printf("  - %s\n", p)
		}
	}

	if len(added) == 0 && len(removed) == 0 {
		fmt.Println("\n✅ No field-level changes detected (descriptions/constraints may differ)")
	}

	fmt.Printf("\nTotal: +%d / -%d field paths\n", len(added), len(removed))
}

func extractPropertyPaths(filePath string) []string {
	// #nosec G703 -- filePath comes from repository-controlled schema paths in this maintenance tool.
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	var obj any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	var paths []string
	collectPaths(obj, "", &paths)
	return paths
}

func collectPaths(obj any, prefix string, paths *[]string) {
	m, ok := obj.(map[string]any)
	if !ok {
		return
	}
	if props, ok := m["properties"].(map[string]any); ok {
		for key, val := range props {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			*paths = append(*paths, path)
			collectPaths(val, path, paths)
		}
	}
	if items, ok := m["items"]; ok {
		collectPaths(items, prefix+"[]", paths)
	}
}

func difference(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, s := range b {
		set[s] = struct{}{}
	}
	var result []string
	for _, s := range a {
		if _, ok := set[s]; !ok {
			result = append(result, s)
		}
	}
	return result
}

func httpGet(url string) ([]byte, error) {
	// #nosec G704 -- URL is constructed from a semver-validated KubeVirt release tag.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	// #nosec G704 -- URL is constructed from a semver-validated KubeVirt release tag.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func readManifest() (*manifest, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func writeManifest(m *manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// #nosec G306 -- manifest is committed repository metadata and should stay world-readable.
	return os.WriteFile(manifestPath, data, 0o644)
}

func fatalf(format string, args ...any) {
	// #nosec G705 -- CLI stderr output is not rendered as HTML and this helper is only used by local maintenance tooling.
	fmt.Fprintf(os.Stderr, "ERROR: %s\n", fmt.Sprintf(format, args...))
	os.Exit(1)
}
