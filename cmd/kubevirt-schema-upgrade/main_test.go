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

func TestSwaggerDownloadURLs(t *testing.T) {
	t.Parallel()

	got := swaggerDownloadURLs("1.8.0")
	want := []string{
		"https://github.com/kubevirt/kubevirt/releases/download/v1.8.0/swagger.json",
		"https://raw.githubusercontent.com/kubevirt/kubevirt/v1.8.0/api/openapi-spec/swagger.json",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("swaggerDownloadURLs() = %#v, want %#v", got, want)
	}
}

func TestUpgradeNextSteps(t *testing.T) {
	t.Parallel()

	got := upgradeNextSteps("1.8.0", "kubevirt-v1.8.0")
	want := []string{
		"Review the diff above",
		"Run 'make kubevirt-schema-report' to inspect added fields and locale gaps",
		"Update instancesize.mask.json only for fields you choose to expose",
		`Update embed_test.go version assertions → "kubevirt-v1.8.0"`,
		"Update go.mod: kubevirt.io/api + kubevirt.io/client-go to v1.8.0",
		"Run 'make test' to verify",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("upgradeNextSteps() = %#v, want %#v", got, want)
	}
}
