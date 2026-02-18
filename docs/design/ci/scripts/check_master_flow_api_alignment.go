//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	masterFlowPath = "docs/design/interaction-flows/master-flow.md"
	openAPIPath    = "api/openapi.yaml"
	allowlistPath  = "docs/design/ci/allowlists/master_flow_api_deferred.txt"
)

var (
	masterAPIRefRe = regexp.MustCompile(`(?i)\b(?:GET|POST|PUT|PATCH|DELETE|WS)\s+(/api/v1[^\s)>,"']+)`)
	openAPIPathRe  = regexp.MustCompile(`^\s{2}(/[^:]+):\s*$`)
	pathParamRe    = regexp.MustCompile(`\{[^}/]+\}`)
)

func main() {
	masterPaths, err := collectMasterFlowPaths(masterFlowPath)
	if err != nil {
		fmt.Printf("FAIL: collect master-flow paths: %v\n", err)
		os.Exit(1)
	}

	specPaths, err := collectOpenAPIPaths(openAPIPath)
	if err != nil {
		fmt.Printf("FAIL: collect openapi paths: %v\n", err)
		os.Exit(1)
	}

	deferredPaths, err := collectAllowlistPaths(allowlistPath)
	if err != nil {
		fmt.Printf("FAIL: collect deferred allowlist paths: %v\n", err)
		os.Exit(1)
	}

	missingInSpec := difference(masterPaths, specPaths)
	undeclaredMissing := subtractSet(missingInSpec, deferredPaths)
	staleAllowlist := staleAllowlistEntries(deferredPaths, missingInSpec)

	if len(undeclaredMissing) > 0 || len(staleAllowlist) > 0 {
		fmt.Println("FAIL: master-flow API alignment check failed")
		if len(undeclaredMissing) > 0 {
			sort.Strings(undeclaredMissing)
			fmt.Println(" - Missing in OpenAPI and NOT declared in deferred allowlist:")
			for _, path := range undeclaredMissing {
				fmt.Printf("   - %s\n", path)
			}
		}
		if len(staleAllowlist) > 0 {
			sort.Strings(staleAllowlist)
			fmt.Println(" - Deferred allowlist contains stale paths (now implemented or removed):")
			for _, path := range staleAllowlist {
				fmt.Printf("   - %s\n", path)
			}
		}
		fmt.Printf("Rule: Every master-flow API path must be implemented in OpenAPI or explicitly tracked in %s\n", allowlistPath)
		os.Exit(1)
	}

	fmt.Printf("OK: master-flow API alignment passed (master=%d, openapi=%d, deferred=%d)\n", len(masterPaths), len(specPaths), len(deferredPaths))
}

func collectMasterFlowPaths(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]struct{})
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		matches := masterAPIRefRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) != 2 {
				continue
			}
			norm := normalizePath(m[1])
			if norm != "" {
				out[norm] = struct{}{}
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func collectOpenAPIPaths(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]struct{})
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		m := openAPIPathRe.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		norm := normalizePath(m[1])
		if norm != "" {
			out[norm] = struct{}{}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func collectAllowlistPaths(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]struct{})
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		norm := normalizePath(line)
		if norm != "" {
			out[norm] = struct{}{}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizePath(in string) string {
	p := strings.TrimSpace(in)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(p), "/api/v1") {
		p = p[len("/api/v1"):]
	}
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSpace(p)
	p = strings.TrimRight(p, ".,;)")
	if p == "" || !strings.HasPrefix(p, "/") {
		return ""
	}
	p = pathParamRe.ReplaceAllString(p, "{}")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func difference(a, b map[string]struct{}) []string {
	out := make([]string, 0)
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}

func subtractSet(paths []string, blocked map[string]struct{}) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := blocked[p]; !ok {
			out = append(out, p)
		}
	}
	return out
}

func staleAllowlistEntries(allowlist map[string]struct{}, missing []string) []string {
	missingSet := make(map[string]struct{}, len(missing))
	for _, p := range missing {
		missingSet[p] = struct{}{}
	}
	out := make([]string, 0)
	for p := range allowlist {
		if _, ok := missingSet[p]; !ok {
			out = append(out, p)
		}
	}
	return out
}
