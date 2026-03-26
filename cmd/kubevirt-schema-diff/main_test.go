package main

import (
	"slices"
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
