//go:build ignore

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

const appRoot = "web/src/app"

var blockedMarkerRe = regexp.MustCompile(`(?i)\b(placeholder[- ]only|stub|not implemented|coming soon)\b`)

func main() {
	files, err := collectRoutePages(appRoot)
	if err != nil {
		fmt.Printf("FAIL: collect route pages: %v\n", err)
		os.Exit(1)
	}

	violations := make([]string, 0)
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: read failed: %v", path, err))
			continue
		}
		if blockedMarkerRe.Match(content) {
			match := blockedMarkerRe.Find(content)
			violations = append(violations, fmt.Sprintf("%s: contains blocked placeholder marker %q", path, string(match)))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: frontend placeholder page marker check failed")
		for _, v := range violations {
			fmt.Printf(" - %s\n", v)
		}
		fmt.Println("Rule: app/**/page.tsx must be production route shells and must not contain placeholder/stub markers.")
		os.Exit(1)
	}

	fmt.Printf("OK: frontend placeholder page marker check passed (routes=%d)\n", len(files))
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
		paths = append(paths, filepath.ToSlash(path))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
