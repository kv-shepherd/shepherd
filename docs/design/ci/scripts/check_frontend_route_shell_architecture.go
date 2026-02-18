//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	appRoot             = "web/src/app"
	allowlistPath       = "docs/design/ci/allowlists/frontend_route_shell_legacy.txt"
	allowlistLockPath   = "docs/design/ci/locks/frontend-route-shell-legacy.lock"
	defaultMaxLines     = 300
	defaultMaxMutations = 0
)

var mutationRe = regexp.MustCompile(`api\.(POST|PUT|PATCH|DELETE)\(`)

type allowance struct {
	maxLines     int
	maxMutations int
}

func main() {
	allowlist, err := loadAllowlist(allowlistPath)
	if err != nil {
		fmt.Printf("FAIL: load allowlist: %v\n", err)
		os.Exit(1)
	}
	lockedPaths, err := loadPathLock(allowlistLockPath)
	if err != nil {
		fmt.Printf("FAIL: load allowlist lock: %v\n", err)
		os.Exit(1)
	}

	files, err := collectRoutePages(appRoot)
	if err != nil {
		fmt.Printf("FAIL: collect route pages: %v\n", err)
		os.Exit(1)
	}

	violations := make([]string, 0)
	usedAllowlist := map[string]bool{}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			violations = append(violations, fmt.Sprintf("cannot read %s: %v", file, err))
			continue
		}

		lines := countLines(content)
		mutations := len(mutationRe.FindAll(content, -1))

		limits := allowance{maxLines: defaultMaxLines, maxMutations: defaultMaxMutations}
		if v, ok := allowlist[file]; ok {
			limits = v
			usedAllowlist[file] = true
			if lines <= defaultMaxLines && mutations <= defaultMaxMutations {
				violations = append(violations, fmt.Sprintf("stale allowlist entry (route already meets default thresholds): %s", file))
			}
		}

		if lines > limits.maxLines {
			violations = append(violations, fmt.Sprintf("route page too large: %s (lines=%d, max=%d)", file, lines, limits.maxLines))
		}
		if mutations > limits.maxMutations {
			violations = append(violations, fmt.Sprintf("route page has too many write API calls: %s (mutations=%d, max=%d)", file, mutations, limits.maxMutations))
		}
	}

	for path := range allowlist {
		if _, ok := lockedPaths[path]; !ok {
			violations = append(violations, fmt.Sprintf("allowlist expansion is forbidden; path not in lock: %s", path))
		}
		if !usedAllowlist[path] {
			violations = append(violations, fmt.Sprintf("stale allowlist path (file missing or not a page): %s", path))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: frontend route shell architecture check failed")
		for _, v := range violations {
			fmt.Printf(" - %s\n", v)
		}
		fmt.Printf("Rule: app/**/page.tsx defaults to <=%d lines and <=%d write API calls; temporary legacy exceptions must be explicit.\n", defaultMaxLines, defaultMaxMutations)
		fmt.Printf("Rule: legacy allowlist cannot add new paths unless lock file is intentionally updated: %s\n", allowlistLockPath)
		os.Exit(1)
	}

	fmt.Printf("OK: frontend route shell architecture check passed (routes=%d allowlist=%d lock=%d)\n", len(files), len(allowlist), len(lockedPaths))
}

func collectRoutePages(root string) ([]string, error) {
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) != "page.tsx" {
			return nil
		}
		path = filepath.ToSlash(path)
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	lines := 1
	for _, b := range content {
		if b == '\n' {
			lines++
		}
	}
	return lines
}

func loadAllowlist(path string) (map[string]allowance, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]allowance{}, nil
		}
		return nil, err
	}
	defer f.Close()

	result := make(map[string]allowance)
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			return nil, fmt.Errorf("%s:%d invalid format, expected path|max_lines|max_mutations", path, lineNo)
		}
		routePath := strings.TrimSpace(parts[0])
		if routePath == "" {
			return nil, fmt.Errorf("%s:%d empty path", path, lineNo)
		}
		maxLines, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || maxLines < 1 {
			return nil, fmt.Errorf("%s:%d invalid max_lines: %q", path, lineNo, parts[1])
		}
		maxMutations, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil || maxMutations < 0 {
			return nil, fmt.Errorf("%s:%d invalid max_mutations: %q", path, lineNo, parts[2])
		}
		result[filepath.ToSlash(routePath)] = allowance{maxLines: maxLines, maxMutations: maxMutations}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func loadPathLock(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	locked := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entryPath := filepath.ToSlash(line)
		if _, exists := locked[entryPath]; exists {
			return nil, fmt.Errorf("%s:%d duplicate lock path: %s", path, lineNo, entryPath)
		}
		locked[entryPath] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return locked, nil
}
