//go:build ignore

package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var violations []string

	_ = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err == nil {
			for _, imp := range file.Imports {
				if imp.Path == nil {
					continue
				}
				raw := strings.Trim(imp.Path.Value, `"`)
				if raw == "github.com/mattn/go-sqlite3" {
					pos := fset.Position(imp.Pos())
					violations = append(violations, fmt.Sprintf("%s:%d imports forbidden sqlite driver %q", path, pos.Line, raw))
				}
			}
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		text := string(content)

		for _, needle := range []string{
			`"sqlite3"`,
			`mode=memory`,
			`go-sqlite3`,
		} {
			if !strings.Contains(text, needle) {
				continue
			}
			line := 1 + strings.Count(text[:strings.Index(text, needle)], "\n")
			violations = append(violations, fmt.Sprintf("%s:%d contains forbidden SQLite marker %q", path, line, needle))
		}

		return nil
	})

	modBytes, err := os.ReadFile("go.mod")
	if err == nil && strings.Contains(string(modBytes), "github.com/mattn/go-sqlite3") {
		violations = append(violations, "go.mod contains forbidden dependency github.com/mattn/go-sqlite3")
	}

	if len(violations) == 0 {
		fmt.Println("OK: no SQLite usage found in tests or module dependencies")
		return
	}

	fmt.Println("FAIL: SQLite usage is forbidden (project is PostgreSQL-only)")
	for _, v := range violations {
		fmt.Println(" -", v)
	}
	os.Exit(1)
}
