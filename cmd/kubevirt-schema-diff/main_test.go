package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCollectMaskPathsTrimsAndDeduplicates(t *testing.T) {
	raw := []byte(`{
		"quick_fields": [{"path":" spec.cpu "}, {"path":"spec.cpu"}],
		"advanced_fields": [{"path":" spec.memory "}],
		"professional_fields": [{"path":"   "}, {"path":"spec.cpu.model"}]
	}`)

	got, err := collectMaskPaths(raw)
	if err != nil {
		t.Fatalf("collectMaskPaths returned error: %v", err)
	}

	want := map[string]struct{}{
		"spec.cpu":       {},
		"spec.memory":    {},
		"spec.cpu.model": {},
	}
	if len(got) != len(want) {
		t.Fatalf("collectMaskPaths size = %d, want %d", len(got), len(want))
	}
	for path := range want {
		if _, ok := got[path]; !ok {
			t.Fatalf("collectMaskPaths missing path %q", path)
		}
	}
}

func TestFlattenSchemaCapturesNestedPaths(t *testing.T) {
	raw := []byte(`{
		"type": "object",
		"properties": {
			"spec": {
				"type": "object",
				"properties": {
					"memory": {
						"type": "object",
						"description": "Guest memory",
						"properties": {
							"guest": {"type": "string", "description": "One of the guest memory targets"}
						}
					},
					"interfaces": {
						"type": "array",
						"items": {
							"type": "object",
							"properties": {
								"name": {"type": "string", "enum": ["default", "mgmt"]}
							}
						}
					},
					"labels": {
						"type": "object",
						"additionalProperties": {"type": "string"}
					}
				}
			}
		}
	}`)

	got, err := flattenSchema(raw)
	if err != nil {
		t.Fatalf("flattenSchema returned error: %v", err)
	}

	for _, path := range []string{
		"spec",
		"spec.memory",
		"spec.memory.guest",
		"spec.interfaces",
		"spec.interfaces[]",
		"spec.interfaces[].name",
		"spec.labels",
		"spec.labels.*",
	} {
		if _, ok := got[path]; !ok {
			t.Fatalf("flattenSchema missing path %q", path)
		}
	}

	if got["spec.interfaces[].name"].Enum != "default,mgmt" {
		t.Fatalf("flattenSchema enum = %q, want %q", got["spec.interfaces[].name"].Enum, "default,mgmt")
	}
}

func TestDiffChangedFieldsSortsOutput(t *testing.T) {
	left := map[string]fieldSnapshot{
		"spec.memory.guest": {Path: "spec.memory.guest", Type: "string", Description: "Guest memory"},
		"spec.cpu.model":    {Path: "spec.cpu.model", Type: "string", Enum: "host-model,host-passthrough"},
	}
	right := map[string]fieldSnapshot{
		"spec.memory.guest": {Path: "spec.memory.guest", Type: "string", Description: "Requested guest memory"},
		"spec.cpu.model":    {Path: "spec.cpu.model", Type: "string", Enum: "host-passthrough"},
	}

	got := diffChangedFields(left, right)
	want := []string{
		"spec.cpu.model\n  type: \"string\" -> \"string\"\n  enum: \"host-model,host-passthrough\" -> \"host-passthrough\"\n  description: \"\" -> \"\"",
		"spec.memory.guest\n  type: \"string\" -> \"string\"\n  enum: \"\" -> \"\"\n  description: \"Guest memory\" -> \"Requested guest memory\"",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("diffChangedFields = %#v, want %#v", got, want)
	}
}

func TestLoadBundleFromDirectory(t *testing.T) {
	dir := t.TempDir()
	schemaBytes := []byte(`{"type":"object","properties":{"spec":{"type":"object"}}}`)
	maskBytes := []byte(`{"quick_fields":[{"path":"spec"}]}`)
	writeFile(t, filepath.Join(dir, "instancesize.schema.json"), schemaBytes)
	writeFile(t, filepath.Join(dir, "instancesize.mask.json"), maskBytes)

	bundle, err := loadBundle("instancesize", "", dir, "from")
	if err != nil {
		t.Fatalf("loadBundle() error = %v", err)
	}
	if bundle.Label != "dir:"+dir {
		t.Fatalf("bundle label = %q, want %q", bundle.Label, "dir:"+dir)
	}
	if !slices.Equal(bundle.Schema, schemaBytes) {
		t.Fatalf("bundle schema = %q, want %q", bundle.Schema, schemaBytes)
	}
	if !slices.Equal(bundle.Mask, maskBytes) {
		t.Fatalf("bundle mask = %q, want %q", bundle.Mask, maskBytes)
	}
}

func TestLoadBundleValidatesSourceSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		versionName string
		dir         string
		want        string
	}{
		{
			name:        "both version and dir",
			versionName: "kubevirt-v1.8.4",
			dir:         "fixtures",
			want:        "from: choose either --from-version or --from-dir",
		},
		{
			name: "neither version nor dir",
			want: "from: one of --from-version or --from-dir is required",
		},
		{
			name:        "unknown embedded version",
			versionName: "kubevirt-v0.0.0",
			want:        `from: unknown embedded version "kubevirt-v0.0.0" for entity "instancesize"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadBundle("instancesize", tc.versionName, tc.dir, "from")
			if err == nil {
				t.Fatal("loadBundle() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadBundle() error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadBundleReportsMissingDirectoryFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "instancesize.schema.json"), []byte(`{"type":"object"}`))

	_, err := loadBundle("instancesize", "", dir, "to")
	if err == nil {
		t.Fatal("loadBundle() error = nil, want missing mask error")
	}
	if !strings.Contains(err.Error(), "to: read "+filepath.Join(dir, "instancesize.mask.json")) {
		t.Fatalf("loadBundle() error = %q, want missing mask path", err.Error())
	}
}

func TestDiffOnlyHelpersSortOutput(t *testing.T) {
	gotKeys := diffOnlyKeys(
		map[string]fieldSnapshot{
			"spec.z": {Path: "spec.z"},
			"spec.a": {Path: "spec.a"},
			"spec.b": {Path: "spec.b"},
		},
		map[string]fieldSnapshot{
			"spec.b": {Path: "spec.b"},
		},
	)
	if !slices.Equal(gotKeys, []string{"spec.a", "spec.z"}) {
		t.Fatalf("diffOnlyKeys() = %#v, want sorted unique paths", gotKeys)
	}

	gotPaths := diffOnlyPaths(
		map[string]struct{}{"spec.z": {}, "spec.a": {}, "spec.b": {}},
		map[string]struct{}{"spec.b": {}},
	)
	if !slices.Equal(gotPaths, []string{"spec.a", "spec.z"}) {
		t.Fatalf("diffOnlyPaths() = %#v, want sorted unique paths", gotPaths)
	}
}

func TestPrintDiffSections(t *testing.T) {
	output := captureStdout(t, func() {
		printPathSetDiff("Mask paths added", []string{"spec.cpu", "spec.memory"})
		printChangedFields("Schema field changes", []string{"spec.cpu\n  type: \"object\" -> \"string\""})
	})

	for _, want := range []string{
		"Mask paths added: 2",
		"- spec.cpu",
		"- spec.memory",
		"Schema field changes: 1",
		"- spec.cpu\n  type: \"object\" -> \"string\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printed output missing %q:\n%s", want, output)
		}
	}
}

func TestScalarStringHandlesCompositeTypes(t *testing.T) {
	got := scalarString([]any{"string", "null", 42})
	if got != "string|null|" {
		t.Fatalf("scalarString() = %q, want %q", got, "string|null|")
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = write
	t.Cleanup(func() {
		os.Stdout = original
	})

	fn()

	if closeErr := write.Close(); closeErr != nil {
		t.Fatalf("close stdout writer: %v", closeErr)
	}
	os.Stdout = original
	data, err := io.ReadAll(read)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if closeErr := read.Close(); closeErr != nil {
		t.Fatalf("close stdout reader: %v", closeErr)
	}
	return string(data)
}
