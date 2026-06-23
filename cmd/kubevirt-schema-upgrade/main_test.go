package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

func TestHTTPGetFromUsesBoundedRequestContext(t *testing.T) {
	var sawDeadline bool
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok {
			t.Fatal("request context missing deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > schemaMaintenanceRequestTimeout {
			t.Fatalf("request context deadline remaining = %s, want within %s", remaining, schemaMaintenanceRequestTimeout)
		}
		sawDeadline = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"swagger":"2.0"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	data, err := httpGetFrom(client, "https://example.test/swagger.json")
	if err != nil {
		t.Fatalf("httpGetFrom() error = %v", err)
	}
	if string(data) != `{"swagger":"2.0"}` {
		t.Fatalf("httpGetFrom() = %q, want swagger body", string(data))
	}
	if !sawDeadline {
		t.Fatal("httpGetFrom() did not execute test transport")
	}
}

func TestHTTPGetFromRejectsOversizedResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"swagger":"` + strings.Repeat("x", 64) + `"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	_, err := httpGetFromWithLimit(client, "https://example.test/swagger.json", 32)
	if err == nil {
		t.Fatal("httpGetFromWithLimit() error = nil, want oversized response error")
	}
	if !strings.Contains(err.Error(), "KubeVirt swagger response exceeds 32 bytes") {
		t.Fatalf("httpGetFromWithLimit() error = %q, want oversized response error", err.Error())
	}
}

func TestExtractVMSpecResolvesTemplateReferences(t *testing.T) {
	swagger := map[string]any{
		"definitions": map[string]any{
			"kubevirt.io.api.core.v1.VirtualMachineSpec": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"template": map[string]any{
						"$ref": "#/definitions/v1.VirtualMachineInstanceTemplateSpec",
					},
				},
			},
			"v1.VirtualMachineInstanceTemplateSpec": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"spec": map[string]any{
						"$ref": "#/definitions/v1.VirtualMachineInstanceSpec",
					},
				},
			},
			"v1.VirtualMachineInstanceSpec": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"domain": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"cpu": map[string]any{"type": "object"},
						},
					},
				},
			},
		},
	}
	data, err := json.Marshal(swagger)
	if err != nil {
		t.Fatalf("marshal swagger: %v", err)
	}

	schema, err := extractVMSpec(data, "1.9.0")
	if err != nil {
		t.Fatalf("extractVMSpec() error = %v", err)
	}
	if got, want := schema["$id"], "kv-shepherd:instancesize:kubevirt-v1.9.0"; got != want {
		t.Fatalf("schema $id = %q, want %q", got, want)
	}

	template := nestedMap(t, schema, "properties", "spec", "properties", "template")
	spec := nestedMap(t, template, "properties", "spec")
	domain := nestedMap(t, spec, "properties", "domain")
	cpu := nestedMap(t, domain, "properties", "cpu")
	if got, want := cpu["type"], "object"; got != want {
		t.Fatalf("resolved cpu type = %q, want %q", got, want)
	}
}

func TestExtractVMSpecReportsMissingDefinitionCandidates(t *testing.T) {
	data := []byte(`{
		"definitions": {
			"v1.VirtualMachineStatus": {"type": "object"},
			"v1.PodSpec": {"type": "object"}
		}
	}`)

	_, err := extractVMSpec(data, "1.9.0")
	if err == nil {
		t.Fatal("extractVMSpec() error = nil, want missing definition error")
	}
	if !strings.Contains(err.Error(), "VirtualMachineSpec not found") {
		t.Fatalf("extractVMSpec() error = %v, want missing spec message", err)
	}
	if !strings.Contains(err.Error(), "v1.VirtualMachineStatus") {
		t.Fatalf("extractVMSpec() error = %v, want VirtualMachine candidate", err)
	}
}

func TestPrintDiffSummaryReportsAddedAndRemovedPaths(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.json")
	newPath := filepath.Join(dir, "new.json")
	writeSchemaFixture(t, oldPath, []string{"spec.cpu", "spec.memory"})
	writeSchemaFixture(t, newPath, []string{"spec.cpu", "spec.disk"})

	output := captureStdout(t, func() {
		printDiffSummary(oldPath, newPath, "old", "new")
	})

	for _, want := range []string{
		"Schema Diff Summary (old → new)",
		"New fields (1):",
		"+ spec.disk",
		"Removed fields (1):",
		"- spec.memory",
		"Total: +1 / -1 field paths",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printDiffSummary() output missing %q:\n%s", want, output)
		}
	}
}

func TestManifestReadWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	manifestDir := filepath.Join(dir, "internal", "pkg", "schema")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if chdirErr := os.Chdir(dir); chdirErr != nil {
		t.Fatalf("chdir temp dir: %v", chdirErr)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(wd); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	})

	want := &manifest{
		Entities: map[string]manifestEntity{
			"instancesize": {
				CurrentVersion: "kubevirt-v1.9.0",
				Versions: map[string]manifestVersion{
					"kubevirt-v1.9.0": {
						KubeVirtVersion: "1.9.0",
						SchemaPath:      "versions/kubevirt-v1.9.0/instancesize.schema.json",
						MaskPath:        "versions/kubevirt-v1.9.0/instancesize.mask.json",
					},
				},
			},
		},
	}
	if writeErr := writeManifest(want); writeErr != nil {
		t.Fatalf("writeManifest() error = %v", writeErr)
	}

	got, err := readManifest()
	if err != nil {
		t.Fatalf("readManifest() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readManifest() = %#v, want %#v", got, want)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read written manifest: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("written manifest missing trailing newline: %q", string(data))
	}
}

func nestedMap(t *testing.T, root map[string]any, keys ...string) map[string]any {
	t.Helper()

	var current any = root
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("path %v reached %T, want object", keys, current)
		}
		current = m[key]
	}
	result, ok := current.(map[string]any)
	if !ok {
		t.Fatalf("path %v reached %T, want object", keys, current)
	}
	return result
}

func writeSchemaFixture(t *testing.T, path string, paths []string) {
	t.Helper()

	root := map[string]any{"properties": map[string]any{}}
	for _, path := range paths {
		current := root["properties"].(map[string]any)
		parts := strings.Split(path, ".")
		for i, part := range parts {
			if i == len(parts)-1 {
				current[part] = map[string]any{}
				continue
			}
			next, ok := current[part].(map[string]any)
			if !ok {
				next = map[string]any{"properties": map[string]any{}}
				current[part] = next
			}
			current = next["properties"].(map[string]any)
		}
	}

	data, err := json.Marshal(root)
	if err != nil {
		t.Fatalf("marshal schema fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write schema fixture: %v", err)
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
	if err := read.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return string(data)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
