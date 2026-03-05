//go:build ignore

// scripts/ci/check_ent_codegen.go

/*
Ent code generation synchronization check - CI enforced

Rules:
1. Run `go generate ./ent` and verify git diff.
2. Differences mean ent/ generated code is out of sync with ent/schema/.
3. Generated files must be committed.

Usage:
  go run scripts/ci/check_ent_codegen.go

Or in CI:
  cd ent && go generate . && git diff --exit-code
*/

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func main() {
	// Ensure ent directory exists.
	if _, err := os.Stat("ent"); os.IsNotExist(err) {
		fmt.Println("WARN: ent/ directory does not exist, skipping check")
		os.Exit(0)
	}

	// Ensure ent/schema directory exists.
	if _, err := os.Stat("ent/schema"); os.IsNotExist(err) {
		fmt.Println("WARN: ent/schema/ directory does not exist, skipping check")
		os.Exit(0)
	}

	fmt.Println("Running go generate ./ent ...")

	// Snapshot workspace state before generation to avoid false positives from pre-existing local changes.
	beforeTracked, err := gitNameOnlyDiff("ent/")
	if err != nil {
		fmt.Printf("ERROR: failed to read pre-generate tracked state: %v\n", err)
		os.Exit(1)
	}
	beforeUntracked, err := gitUntracked("ent/")
	if err != nil {
		fmt.Printf("ERROR: failed to read pre-generate untracked state: %v\n", err)
		os.Exit(1)
	}

	// Run go generate.
	generateCmd := exec.Command("go", "generate", "./ent")
	generateCmd.Stdout = os.Stdout
	generateCmd.Stderr = os.Stderr
	if err := generateCmd.Run(); err != nil {
		fmt.Printf("ERROR: go generate failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Checking ent/ for uncommitted changes...")

	afterTracked, err := gitNameOnlyDiff("ent/")
	if err != nil {
		fmt.Printf("ERROR: failed to read post-generate tracked state: %v\n", err)
		os.Exit(1)
	}
	afterUntracked, err := gitUntracked("ent/")
	if err != nil {
		fmt.Printf("ERROR: failed to read post-generate untracked state: %v\n", err)
		os.Exit(1)
	}

	newTracked := diffSet(afterTracked, beforeTracked)
	newUntracked := diffSet(afterUntracked, beforeUntracked)

	if len(newTracked) > 0 {
		fmt.Println("ERROR: Ent generated code is out of sync")
		fmt.Println("\nThe following files must be regenerated and committed:")
		sort.Strings(newTracked)
		for _, file := range newTracked {
			fmt.Printf("  - %s\n", file)
		}
		fmt.Println("\nFix:")
		fmt.Println("  1. Run: go generate ./ent")
		fmt.Println("  2. Commit generated files: git add ent/ && git commit")
		os.Exit(1)
	}

	if len(newUntracked) > 0 {
		sort.Strings(newUntracked)
		fmt.Println("ERROR: ent/ has new untracked files")
		fmt.Println("\nPlease add and commit these files:")
		for _, file := range newUntracked {
			fmt.Printf("  - %s\n", file)
		}
		os.Exit(1)
	}

	fmt.Println("OK: Ent code generation synchronization check passed")
}

func gitNameOnlyDiff(path string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return splitLines(out.String()), nil
}

func gitUntracked(path string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return splitLines(out.String()), nil
}

func splitLines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func diffSet(after, before []string) []string {
	if len(after) == 0 {
		return nil
	}
	beforeSet := make(map[string]struct{}, len(before))
	for _, item := range before {
		beforeSet[item] = struct{}{}
	}
	out := make([]string, 0, len(after))
	for _, item := range after {
		if _, ok := beforeSet[item]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}
