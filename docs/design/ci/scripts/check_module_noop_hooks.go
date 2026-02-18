//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	modulesDir    = "internal/app/modules"
	allowlistPath = "docs/design/ci/allowlists/module_noop_hooks.txt"
)

var targetMethods = map[string]struct{}{
	"ContributeServerDeps": {},
	"RegisterWorkers":      {},
}

var skipFiles = map[string]struct{}{
	"module.go":         {},
	"infrastructure.go": {},
	"server_deps.go":    {},
}

func main() {
	allowlist, err := loadAllowlist(allowlistPath)
	if err != nil {
		fmt.Printf("FAIL: load allowlist: %v\n", err)
		os.Exit(1)
	}

	matches := map[string]struct{}{}
	var violations []string

	files, err := filepath.Glob(filepath.Join(modulesDir, "*.go"))
	if err != nil {
		fmt.Printf("FAIL: list module files: %v\n", err)
		os.Exit(1)
	}

	for _, path := range files {
		if _, skip := skipFiles[filepath.Base(path)]; skip {
			continue
		}

		fileViolations, fileMatches, err := checkModuleFile(path, allowlist)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: parse failed: %v", path, err))
			continue
		}
		violations = append(violations, fileViolations...)
		for _, m := range fileMatches {
			matches[m] = struct{}{}
		}
	}

	for key := range allowlist {
		if _, ok := matches[key]; !ok {
			violations = append(violations, fmt.Sprintf("stale allowlist entry: %s", key))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: module noop-hook check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Printf("Rule: noop module hooks require explicit allowlist in %s.\n", allowlistPath)
		os.Exit(1)
	}

	fmt.Println("OK: module noop-hook check passed")
}

func checkModuleFile(path string, allowlist map[string]struct{}) ([]string, []string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, err
	}

	var violations []string
	var matches []string

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Body == nil {
			continue
		}
		if _, ok := targetMethods[fn.Name.Name]; !ok {
			continue
		}
		if !isNoopBody(fn.Body) {
			continue
		}

		key := fmt.Sprintf("%s:%s", filepath.ToSlash(path), fn.Name.Name)
		if _, ok := allowlist[key]; !ok {
			pos := fset.Position(fn.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: noop hook without allowlist: %s", path, pos.Line, key))
			continue
		}
		matches = append(matches, key)
	}

	return violations, matches, nil
}

func isNoopBody(body *ast.BlockStmt) bool {
	if body == nil {
		return true
	}
	if len(body.List) == 0 {
		return true
	}
	if len(body.List) == 1 {
		if ret, ok := body.List[0].(*ast.ReturnStmt); ok && len(ret.Results) == 0 {
			return true
		}
	}
	return false
}

func loadAllowlist(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
