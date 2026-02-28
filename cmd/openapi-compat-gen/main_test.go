package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateCompat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		wantErr       string
		wantConverted int
		validate      func(t *testing.T, doc *yaml.Node)
	}{
		{
			name: "inline union-null is converted",
			input: `
openapi: 3.1.0
info: {title: t, version: v}
paths: {}
components:
  schemas:
    Demo:
      type: object
      properties:
        field:
          type: ["string", "null"]
`,
			wantConverted: 1,
			validate: func(t *testing.T, doc *yaml.Node) {
				t.Helper()
				root := documentRoot(t, doc)
				assertTopOpenAPIVersion(t, root, "3.0.3")

				field := lookupPath(t, root, "components", "schemas", "Demo", "properties", "field")
				assertTypeAndNullable(t, field, "string", "true")
			},
		},
		{
			name: "block-style union-null is converted",
			input: `
openapi: 3.1.0
info: {title: t, version: v}
paths: {}
components:
  schemas:
    Demo:
      type: object
      properties:
        field:
          type:
            - string
            - 'null'
`,
			wantConverted: 1,
			validate: func(t *testing.T, doc *yaml.Node) {
				t.Helper()
				root := documentRoot(t, doc)
				field := lookupPath(t, root, "components", "schemas", "Demo", "properties", "field")
				assertTypeAndNullable(t, field, "string", "true")
			},
		},
		{
			name: "three-element union fails closed",
			input: `
openapi: 3.1.0
info: {title: t, version: v}
paths: {}
components:
  schemas:
    Demo:
      type: object
      properties:
        field:
          type: ["string", "integer", "null"]
`,
			wantErr: "unsupported type union",
		},
		{
			name: "two-element non-null union fails closed",
			input: `
openapi: 3.1.0
info: {title: t, version: v}
paths: {}
components:
  schemas:
    Demo:
      type: object
      properties:
        field:
          type: ["string", "integer"]
`,
			wantErr: "unsupported type union",
		},
		{
			name: "3.1-only keyword fails closed",
			input: `
openapi: 3.1.0
info: {title: t, version: v}
paths: {}
components:
  schemas:
    Demo:
      type: object
      prefixItems:
        - type: string
`,
			wantErr: "unsupported OpenAPI 3.1 keyword",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, converted, err := generateCompat([]byte(tt.input))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("generateCompat returned unexpected error: %v", err)
			}
			if converted != tt.wantConverted {
				t.Fatalf("converted = %d, want %d", converted, tt.wantConverted)
			}

			var doc yaml.Node
			if err := yaml.Unmarshal(out, &doc); err != nil {
				t.Fatalf("failed to parse output yaml: %v", err)
			}
			tt.validate(t, &doc)
		})
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("usage error", func(t *testing.T) {
		t.Parallel()
		err := run(nil, os.Stdout, os.Stderr)
		if err == nil {
			t.Fatal("expected usage error, got nil")
		}
		if !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("expected usage error, got: %v", err)
		}
	})

	t.Run("writes compat file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		inPath := filepath.Join(dir, "openapi.yaml")
		outPath := filepath.Join(dir, "openapi.compat.yaml")
		input := `
openapi: 3.1.0
info: {title: t, version: v}
paths: {}
components:
  schemas:
    Demo:
      type: object
      properties:
        field:
          type: ["string", "null"]
`
		if err := os.WriteFile(inPath, []byte(input), 0o644); err != nil {
			t.Fatalf("write input: %v", err)
		}
		if err := run([]string{inPath, outPath}, os.Stdout, os.Stderr); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
		out, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		if !strings.Contains(string(out), "nullable: true") {
			t.Fatalf("output missing nullable conversion:\n%s", string(out))
		}
		if !strings.Contains(string(out), "openapi: 3.0.3") {
			t.Fatalf("output missing openapi version rewrite:\n%s", string(out))
		}
	})
}

func documentRoot(t *testing.T, doc *yaml.Node) *yaml.Node {
	t.Helper()
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		t.Fatalf("invalid yaml document")
	}
	return doc.Content[0]
}

func lookupPath(t *testing.T, node *yaml.Node, keys ...string) *yaml.Node {
	t.Helper()
	current := node
	for _, key := range keys {
		current = findMappingValue(current, key)
		if current == nil {
			t.Fatalf("missing key in path: %s", strings.Join(keys, "."))
		}
	}
	return current
}

func assertTopOpenAPIVersion(t *testing.T, root *yaml.Node, want string) {
	t.Helper()
	openapi := findMappingValue(root, "openapi")
	if openapi == nil {
		t.Fatalf("missing top-level openapi field")
	}
	if openapi.Value != want {
		t.Fatalf("openapi = %q, want %q", openapi.Value, want)
	}
}

func assertTypeAndNullable(t *testing.T, node *yaml.Node, wantType, wantNullable string) {
	t.Helper()
	typeNode := findMappingValue(node, "type")
	if typeNode == nil {
		t.Fatalf("missing type field")
	}
	if typeNode.Kind != yaml.ScalarNode {
		t.Fatalf("type field should be scalar, got kind=%d", typeNode.Kind)
	}
	if typeNode.Value != wantType {
		t.Fatalf("type = %q, want %q", typeNode.Value, wantType)
	}

	nullableNode := findMappingValue(node, "nullable")
	if nullableNode == nil {
		t.Fatalf("missing nullable field")
	}
	if !strings.EqualFold(nullableNode.Value, wantNullable) {
		t.Fatalf("nullable = %q, want %q", nullableNode.Value, wantNullable)
	}
}
