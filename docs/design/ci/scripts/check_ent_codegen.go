//go:build ignore

// docs/design/ci/scripts/check_ent_codegen.go

/*
Ent code generation synchronization check - CI enforced

Rules:
1. Snapshot tracked/untracked Ent file contents, run `go generate ./ent`, and compare content hashes.
2. Differences mean ent/ generated code is out of sync with ent/schema/.
3. Generated files must be committed.

Usage:
  go run docs/design/ci/scripts/check_ent_codegen.go

Or in CI:
  cd ent && go generate . && git diff --exit-code
*/

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func main() {
	if err := checkSnapshotDiffGuard(); err != nil {
		fmt.Printf("ERROR: Ent codegen content-snapshot self-test failed: %v\n", err)
		os.Exit(1)
	}

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

	// Snapshot file contents before generation. Comparing only changed filenames is
	// insufficient in a dirty tree: a generator can change an already-dirty file
	// while the name-only diff remains identical.
	before, err := snapshotEntFiles()
	if err != nil {
		fmt.Printf("ERROR: failed to snapshot pre-generate Ent contents: %v\n", err)
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

	after, err := snapshotEntFiles()
	if err != nil {
		fmt.Printf("ERROR: failed to snapshot post-generate Ent contents: %v\n", err)
		os.Exit(1)
	}

	changed := changedSnapshotPaths(before, after)
	if len(changed) > 0 {
		fmt.Println("ERROR: Ent generated code is out of sync")
		fmt.Println("\nThe generator changed the following files:")
		for _, file := range changed {
			fmt.Printf("  - %s\n", file)
		}
		fmt.Println("\nFix:")
		fmt.Println("  1. Run: go generate ./ent")
		fmt.Println("  2. Commit generated files: git add ent/ && git commit")
		os.Exit(1)
	}

	fmt.Println("OK: Ent code generation synchronization check passed")
}

type contentSnapshot map[string][sha256.Size]byte

func snapshotEntFiles() (contentSnapshot, error) {
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "--", "ent/")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	paths := splitLines(out.String())
	snapshot := make(contentSnapshot, len(paths))
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		snapshot[path] = sha256.Sum256(contents)
	}
	return snapshot, nil
}

func changedSnapshotPaths(before, after contentSnapshot) []string {
	paths := make(map[string]struct{}, len(before)+len(after))
	for path := range before {
		paths[path] = struct{}{}
	}
	for path := range after {
		paths[path] = struct{}{}
	}

	changed := make([]string, 0)
	for path := range paths {
		beforeHash, existedBefore := before[path]
		afterHash, existsAfter := after[path]
		if !existedBefore || !existsAfter || beforeHash != afterHash {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
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

func checkSnapshotDiffGuard() error {
	unchanged := sha256.Sum256([]byte("unchanged"))
	beforeValue := sha256.Sum256([]byte("before"))
	afterValue := sha256.Sum256([]byte("after"))
	before := contentSnapshot{
		"ent/unchanged.go": unchanged,
		"ent/changed.go":   beforeValue,
		"ent/deleted.go":   beforeValue,
	}
	after := contentSnapshot{
		"ent/unchanged.go": unchanged,
		"ent/changed.go":   afterValue,
		"ent/added.go":     afterValue,
	}
	want := "ent/added.go,ent/changed.go,ent/deleted.go"
	if got := strings.Join(changedSnapshotPaths(before, after), ","); got != want {
		return fmt.Errorf("changed paths = %q, want %q", got, want)
	}
	if got := changedSnapshotPaths(before, before); len(got) != 0 {
		return fmt.Errorf("identical snapshots reported changes: %v", got)
	}
	return nil
}
