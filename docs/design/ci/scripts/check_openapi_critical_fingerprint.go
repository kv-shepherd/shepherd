//go:build ignore

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	specPath = "api/openapi.yaml"
	lockPath = "docs/design/ci/locks/openapi-critical.lock"
)

func main() {
	writeLock := flag.Bool("write-lock", false, "write fingerprint lock file from current api/openapi.yaml")
	flag.Parse()

	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", specPath, err)
		os.Exit(1)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(specBytes, &doc); err != nil {
		fmt.Printf("FAIL: parse %s: %v\n", specPath, err)
		os.Exit(1)
	}
	root := documentRoot(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		fmt.Printf("FAIL: %s root must be mapping\n", specPath)
		os.Exit(1)
	}

	expected, err := buildFingerprint(root)
	if err != nil {
		fmt.Printf("FAIL: build fingerprint: %v\n", err)
		os.Exit(1)
	}

	if *writeLock {
		if err := writeFingerprintLock(lockPath, expected); err != nil {
			fmt.Printf("FAIL: write lock: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("OK: wrote %s\n", lockPath)
		return
	}

	actual, err := readFingerprintLock(lockPath)
	if err != nil {
		fmt.Printf("FAIL: read lock: %v\n", err)
		os.Exit(1)
	}

	var violations []string
	for k, v := range expected {
		got, ok := actual[k]
		if !ok {
			violations = append(violations, fmt.Sprintf("missing lock key: %s", k))
			continue
		}
		if got != v {
			violations = append(violations, fmt.Sprintf("fingerprint mismatch for %s: lock=%s current=%s", k, got, v))
		}
	}
	for k := range actual {
		if _, ok := expected[k]; !ok {
			violations = append(violations, fmt.Sprintf("stale lock key: %s", k))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: OpenAPI critical fingerprint check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Printf("If changes are intentional, run:\n  go run docs/design/ci/scripts/check_openapi_critical_fingerprint.go -write-lock\n")
		os.Exit(1)
	}

	fmt.Println("OK: OpenAPI critical fingerprint check passed")
}

func buildFingerprint(root *yaml.Node) (map[string]string, error) {
	type source struct {
		key  string
		node *yaml.Node
	}

	var sources []source

	components, ok := mapValue(root, "components")
	if !ok {
		return nil, fmt.Errorf("missing root.components")
	}
	schemas, ok := mapValue(components, "schemas")
	if !ok {
		return nil, fmt.Errorf("missing components.schemas")
	}
	securitySchemes, ok := mapValue(components, "securitySchemes")
	if !ok {
		return nil, fmt.Errorf("missing components.securitySchemes")
	}
	paths, ok := mapValue(root, "paths")
	if !ok {
		return nil, fmt.Errorf("missing root.paths")
	}
	globalSecurity, ok := mapValue(root, "security")
	if !ok {
		return nil, fmt.Errorf("missing root.security")
	}

	requiredNodes := []struct {
		key string
		get func() (*yaml.Node, bool)
	}{
		{
			key: "root.security",
			get: func() (*yaml.Node, bool) { return globalSecurity, true },
		},
		{
			key: "components.securitySchemes.BearerAuth",
			get: func() (*yaml.Node, bool) { return mapValue(securitySchemes, "BearerAuth") },
		},
		{
			key: "components.schemas.Notification",
			get: func() (*yaml.Node, bool) { return mapValue(schemas, "Notification") },
		},
		{
			key: "components.schemas.NotificationList",
			get: func() (*yaml.Node, bool) { return mapValue(schemas, "NotificationList") },
		},
		{
			key: "components.schemas.UnreadCount",
			get: func() (*yaml.Node, bool) { return mapValue(schemas, "UnreadCount") },
		},
		{
			key: "components.schemas.VMConsoleRequestResponse",
			get: func() (*yaml.Node, bool) { return mapValue(schemas, "VMConsoleRequestResponse") },
		},
		{
			key: "components.schemas.VMConsoleStatusResponse",
			get: func() (*yaml.Node, bool) { return mapValue(schemas, "VMConsoleStatusResponse") },
		},
		{
			key: "components.schemas.VMVNCSessionResponse",
			get: func() (*yaml.Node, bool) { return mapValue(schemas, "VMVNCSessionResponse") },
		},
		{
			key: "paths./notifications.get",
			get: func() (*yaml.Node, bool) {
				if p, ok := mapValue(paths, "/notifications"); ok {
					return mapValue(p, "get")
				}
				return nil, false
			},
		},
		{
			key: "paths./notifications/unread-count.get",
			get: func() (*yaml.Node, bool) {
				if p, ok := mapValue(paths, "/notifications/unread-count"); ok {
					return mapValue(p, "get")
				}
				return nil, false
			},
		},
		{
			key: "paths./notifications/{notification_id}/read.patch",
			get: func() (*yaml.Node, bool) {
				if p, ok := mapValue(paths, "/notifications/{notification_id}/read"); ok {
					return mapValue(p, "patch")
				}
				return nil, false
			},
		},
		{
			key: "paths./notifications/mark-all-read.post",
			get: func() (*yaml.Node, bool) {
				if p, ok := mapValue(paths, "/notifications/mark-all-read"); ok {
					return mapValue(p, "post")
				}
				return nil, false
			},
		},
		{
			key: "paths./vms/{vm_id}/console/request.post",
			get: func() (*yaml.Node, bool) {
				if p, ok := mapValue(paths, "/vms/{vm_id}/console/request"); ok {
					return mapValue(p, "post")
				}
				return nil, false
			},
		},
		{
			key: "paths./vms/{vm_id}/console/status.get",
			get: func() (*yaml.Node, bool) {
				if p, ok := mapValue(paths, "/vms/{vm_id}/console/status"); ok {
					return mapValue(p, "get")
				}
				return nil, false
			},
		},
		{
			key: "paths./vms/{vm_id}/vnc.get",
			get: func() (*yaml.Node, bool) {
				if p, ok := mapValue(paths, "/vms/{vm_id}/vnc"); ok {
					return mapValue(p, "get")
				}
				return nil, false
			},
		},
	}

	for _, item := range requiredNodes {
		node, ok := item.get()
		if !ok || node == nil {
			return nil, fmt.Errorf("missing critical node: %s", item.key)
		}
		sources = append(sources, source{key: item.key, node: node})
	}

	out := make(map[string]string, len(sources))
	for _, s := range sources {
		out[s.key] = hashNode(s.node)
	}
	return out, nil
}

func hashNode(node *yaml.Node) string {
	canonical := canonicalNode(documentRoot(node))
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func canonicalNode(node *yaml.Node) string {
	if node == nil {
		return "nil"
	}
	node = documentRoot(node)
	if node == nil {
		return "nil"
	}

	switch node.Kind {
	case yaml.ScalarNode:
		return "S(" + node.Tag + "):" + node.Value
	case yaml.SequenceNode:
		parts := make([]string, 0, len(node.Content))
		for _, c := range node.Content {
			parts = append(parts, canonicalNode(c))
		}
		return "Q[" + strings.Join(parts, ",") + "]"
	case yaml.MappingNode:
		type pair struct {
			k string
			v string
		}
		pairs := make([]pair, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			k := node.Content[i]
			v := node.Content[i+1]
			pairs = append(pairs, pair{k: k.Value, v: canonicalNode(v)})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
		parts := make([]string, 0, len(pairs))
		for _, p := range pairs {
			parts = append(parts, p.k+"="+p.v)
		}
		return "M{" + strings.Join(parts, ";") + "}"
	case yaml.AliasNode:
		return "A(" + canonicalNode(node.Alias) + ")"
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return "D{}"
		}
		return canonicalNode(node.Content[0])
	default:
		return fmt.Sprintf("K%d", node.Kind)
	}
}

func writeFingerprintLock(path string, kv map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = fmt.Fprintln(f, "# OpenAPI critical fingerprint lock.")
	_, _ = fmt.Fprintln(f, "# Update command:")
	_, _ = fmt.Fprintln(f, "#   go run docs/design/ci/scripts/check_openapi_critical_fingerprint.go -write-lock")

	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if _, err := fmt.Fprintf(f, "%s=%s\n", k, kv[k]); err != nil {
			return err
		}
	}
	return nil
}

func readFingerprintLock(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line in lock: %q", line)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" || val == "" {
			return nil, fmt.Errorf("invalid key/value in lock line: %q", line)
		}
		out[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc == nil {
		return nil
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func mapValue(node *yaml.Node, key string) (*yaml.Node, bool) {
	node = documentRoot(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return v, true
		}
	}
	return nil, false
}
