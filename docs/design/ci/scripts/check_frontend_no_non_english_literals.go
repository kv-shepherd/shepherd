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
	"strings"
)

const (
	frontendRoot = "web/src"
	localesDir   = "web/src/i18n/locales"
)

var nonEnglishPattern = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]`)

func main() {
	var violations []string

	err := filepath.WalkDir(frontendRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if sameOrSubpath(path, localesDir) {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".ts", ".tsx", ".js", ".jsx", ".json":
		default:
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if !nonEnglishPattern.MatchString(line) {
				continue
			}
			violations = append(violations, fmt.Sprintf("%s:%d: %s", path, lineNo, truncate(line, 140)))
		}
		return scanner.Err()
	})
	if err != nil {
		fmt.Printf("FAIL: scan frontend files: %v\n", err)
		os.Exit(1)
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: frontend non-English literal check failed")
		for _, v := range violations {
			fmt.Printf(" - %s\n", v)
		}
		fmt.Println("Rule: non-English literals must be defined in web/src/i18n/locales and referenced via i18n keys.")
		os.Exit(1)
	}

	fmt.Println("OK: frontend non-English literal check passed")
}

func sameOrSubpath(path, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	return cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
