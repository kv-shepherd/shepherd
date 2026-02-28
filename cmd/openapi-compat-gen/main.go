package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var blockedKeywords = map[string]struct{}{
	"jsonSchemaDialect":     {},
	"unevaluatedProperties": {},
	"dependentSchemas":      {},
	"prefixItems":           {},
	"minContains":           {},
	"maxContains":           {},
	"contentEncoding":       {},
	"contentMediaType":      {},
	"$dynamicRef":           {},
	"$dynamicAnchor":        {},
	"if":                    {},
	"then":                  {},
	"else":                  {},
	"const":                 {},
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: openapi-compat-gen <canonical-spec-path> <compat-spec-path>")
	}
	inPath := args[0]
	outPath := args[1]

	in, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read canonical spec %q: %w", inPath, err)
	}
	out, converted, err := generateCompat(in)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return fmt.Errorf("write compat spec %q: %w", outPath, err)
	}
	_, _ = fmt.Fprintf(stdout, "✅ Compat spec generated: %s (rewrote %d union-null declarations)\n", outPath, converted)
	return nil
}

func generateCompat(in []byte) ([]byte, int, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(in, &doc); err != nil {
		return nil, 0, fmt.Errorf("parse yaml: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, 0, errors.New("yaml document is empty or invalid")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, 0, fmt.Errorf("top-level yaml must be mapping, got kind=%d", root.Kind)
	}

	if err := rewriteOpenAPIVersion(root); err != nil {
		return nil, 0, err
	}

	converted := 0
	if err := transformNode(root, &converted); err != nil {
		return nil, 0, err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, 0, fmt.Errorf("encode compat yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, 0, fmt.Errorf("close yaml encoder: %w", err)
	}

	return buf.Bytes(), converted, nil
}

func rewriteOpenAPIVersion(root *yaml.Node) error {
	val := findMappingValue(root, "openapi")
	if val == nil {
		return errors.New("missing top-level 'openapi' field")
	}
	if val.Kind != yaml.ScalarNode {
		return errors.New("top-level 'openapi' must be scalar")
	}
	val.Tag = "!!str"
	val.Value = "3.0.3"
	return nil
}

func transformNode(node *yaml.Node, converted *int) error {
	switch node.Kind {
	case yaml.MappingNode:
		return transformMapping(node, converted)
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, child := range node.Content {
			if err := transformNode(child, converted); err != nil {
				return err
			}
		}
	case yaml.AliasNode:
		if node.Alias != nil {
			return transformNode(node.Alias, converted)
		}
	}
	return nil
}

func transformMapping(mapping *yaml.Node, converted *int) error {
	if len(mapping.Content)%2 != 0 {
		return fmt.Errorf("invalid mapping node with odd content length at line %d", mapping.Line)
	}

	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		val := mapping.Content[i+1]

		if key.Kind == yaml.ScalarNode {
			if _, blocked := blockedKeywords[key.Value]; blocked {
				return fmt.Errorf("unsupported OpenAPI 3.1 keyword %q at line %d", key.Value, key.Line)
			}
			if key.Value == "type" {
				changed, err := convertTypeUnion(mapping, i)
				if err != nil {
					return err
				}
				if changed {
					(*converted)++
					// nullable pair is inserted after the current key/value pair
					i += 2
				}
			}
		}

		if err := transformNode(val, converted); err != nil {
			return err
		}
	}
	return nil
}

func convertTypeUnion(mapping *yaml.Node, keyIndex int) (bool, error) {
	typeNode := mapping.Content[keyIndex+1]
	if typeNode.Kind != yaml.SequenceNode {
		return false, nil
	}

	if len(typeNode.Content) != 2 {
		return false, fmt.Errorf("unsupported type union (need exactly 2 elements) at line %d", typeNode.Line)
	}

	left := typeNode.Content[0]
	right := typeNode.Content[1]

	leftType, ok := scalarText(left)
	if !ok {
		return false, fmt.Errorf("unsupported non-scalar type union element at line %d", left.Line)
	}
	rightType, ok := scalarText(right)
	if !ok {
		return false, fmt.Errorf("unsupported non-scalar type union element at line %d", right.Line)
	}

	var base *yaml.Node
	leftIsNull := strings.EqualFold(leftType, "null")
	rightIsNull := strings.EqualFold(rightType, "null")
	switch {
	case leftIsNull && !rightIsNull:
		base = right
	case !leftIsNull && rightIsNull:
		base = left
	default:
		return false, fmt.Errorf("unsupported type union %q,%q at line %d", leftType, rightType, typeNode.Line)
	}

	// Convert: type: [T, null] -> type: T
	typeNode.Kind = yaml.ScalarNode
	typeNode.Tag = "!!str"
	typeNode.Style = base.Style
	typeNode.Value = base.Value
	typeNode.Content = nil
	typeNode.Alias = nil

	nullable := findMappingValue(mapping, "nullable")
	if nullable == nil {
		insertAt := keyIndex + 2
		nullableKey := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "nullable"}
		nullableVal := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
		mapping.Content = append(
			mapping.Content[:insertAt],
			append([]*yaml.Node{nullableKey, nullableVal}, mapping.Content[insertAt:]...)...,
		)
		return true, nil
	}

	nullable.Kind = yaml.ScalarNode
	nullable.Tag = "!!bool"
	nullable.Value = "true"
	nullable.Content = nil
	nullable.Alias = nil
	return true, nil
}

func findMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		v := mapping.Content[i+1]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return v
		}
	}
	return nil
}

func scalarText(node *yaml.Node) (string, bool) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return "", false
	}
	text := strings.TrimSpace(node.Value)
	if text == "" {
		return "", false
	}
	return text, true
}
