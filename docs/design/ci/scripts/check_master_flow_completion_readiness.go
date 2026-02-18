//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

type deferredFile struct {
	Path string
	Desc string
}

var deferredFiles = []deferredFile{
	{
		Path: "docs/design/ci/allowlists/master_flow_api_deferred.txt",
		Desc: "master-flow APIs declared but not implemented in OpenAPI",
	},
	{
		Path: "docs/design/ci/allowlists/master_flow_test_deferred.txt",
		Desc: "master-flow required stage tests deferred",
	},
	{
		Path: "docs/design/ci/allowlists/frontend_openapi_unused.txt",
		Desc: "backend operations not consumed by frontend",
	},
	{
		Path: "docs/design/ci/allowlists/test_delta_guard_exempt.txt",
		Desc: "runtime files exempted from strict changed-code-has-tests guard",
	},
	{
		Path: "docs/design/ci/allowlists/frontend_route_shell_legacy.txt",
		Desc: "legacy frontend route-shell architecture exemptions",
	},
	{
		Path: "docs/design/ci/allowlists/module_noop_hooks.txt",
		Desc: "module noop hook exemptions",
	},
}

func main() {
	type item struct {
		file  deferredFile
		lines []string
	}

	var pending []item

	for _, f := range deferredFiles {
		lines, err := loadMeaningfulLines(f.Path)
		if err != nil {
			fmt.Printf("FAIL: read %s: %v\n", f.Path, err)
			os.Exit(1)
		}
		if len(lines) == 0 {
			continue
		}
		pending = append(pending, item{file: f, lines: lines})
	}

	if len(pending) > 0 {
		sort.Slice(pending, func(i, j int) bool {
			return pending[i].file.Path < pending[j].file.Path
		})
		fmt.Println("FAIL: master-flow completion readiness check failed")
		fmt.Println("The following deferred/exemption lists are not empty:")
		for _, p := range pending {
			fmt.Printf(" - %s (%s): %d pending entries\n", p.file.Path, p.file.Desc, len(p.lines))
		}
		fmt.Println("Rule: To claim full master-flow completion, all deferred/exemption allowlists above must be empty.")
		os.Exit(1)
	}

	fmt.Println("OK: master-flow completion readiness check passed (no deferred/exemption entries)")
}

func loadMeaningfulLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}
