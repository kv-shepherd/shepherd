package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDifference(t *testing.T) {
	t.Parallel()

	got := difference([]string{"a", "b", "c"}, []string{"b"})
	want := []string{"a", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("difference() = %#v, want %#v", got, want)
	}
}

func TestExtractPropertyPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "schema.json")
	data := []byte(`{
		"properties": {
			"spec": {
				"properties": {
					"template": {
						"properties": {
							"metadata": {}
						}
					}
				}
			}
		}
	}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	got := extractPropertyPaths(path)
	want := []string{"spec", "spec.template", "spec.template.metadata"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractPropertyPaths() = %#v, want %#v", got, want)
	}
}
