//go:build ignore

// check_workflow_make_parity.go — Thin Go policy checker for workflow/Makefile parity.
//
// This checker enforces repo-specific custom rules that actionlint does not cover:
//  1. Makefile entry-point parity (required `make` targets must appear).
//  2. SHA + `# vX.Y.Z` pinning enforcement on all third-party `uses:` lines.
//  3. Deferral table cross-reference (registered `run: |` exceptions).
//  4. New/unregistered `run: |` blocks prohibition.
//
// Usage: go run docs/design/ci/scripts/check_workflow_make_parity.go [--test-fixture <path>]
//
// The --test-fixture flag checks a single file against rules 2-4 only (for fixture testing).

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ─── Data Structures ───────────────────────────────────────────────────────

// DeferralEntry represents one registered exception in parity-deferral.yaml.
type DeferralEntry struct {
	File      string `yaml:"file"`
	Job       string `yaml:"job"`
	StepName  string `yaml:"step_name"`
	Owner     string `yaml:"owner"`
	Expiry    string `yaml:"expiry"`
	Reason    string `yaml:"reason"`
	Migrating bool   `yaml:"migrating,omitempty"`
}

// DeferralTable is the top-level structure of parity-deferral.yaml.
type DeferralTable struct {
	Deferrals []DeferralEntry `yaml:"deferrals"`
}

// WorkflowFile represents a parsed GitHub Actions workflow YAML.
type WorkflowFile struct {
	Permissions interface{}            `yaml:"permissions"`
	Jobs        map[string]WorkflowJob `yaml:"jobs"`
}

// WorkflowJob represents a single job in a workflow.
type WorkflowJob struct {
	Steps []WorkflowStep `yaml:"steps"`
}

// WorkflowStep represents a single step in a job.
type WorkflowStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
	Uses string `yaml:"uses"`
}

// ─── Constants ─────────────────────────────────────────────────────────────

const (
	workflowDir  = ".github/workflows"
	deferralPath = "docs/design/ci/scripts/parity-deferral.yaml"
)

// ─── Main ──────────────────────────────────────────────────────────────────

func main() {
	// Handle --test-fixture mode
	if len(os.Args) >= 3 && os.Args[1] == "--test-fixture" {
		fixturePath := os.Args[2]
		if err := checkFixture(fixturePath); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("OK: fixture passed")
		os.Exit(0)
	}

	failures := 0

	// 1. Load deferral table
	deferrals, err := loadDeferralTable(deferralPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: cannot load deferral table: %v\n", err)
		os.Exit(1)
	}

	// 2. Validate deferral entries (required fields, duplicates, ≤90 days, expiry)
	today := time.Now().Truncate(24 * time.Hour)
	maxExpiry := today.AddDate(0, 0, 90)
	seen := make(map[string]bool)
	for i, d := range deferrals.Deferrals {
		// Required fields check
		if d.File == "" || d.Job == "" || d.StepName == "" || d.Owner == "" || d.Reason == "" || d.Expiry == "" {
			missing := []string{}
			if d.File == "" {
				missing = append(missing, "file")
			}
			if d.Job == "" {
				missing = append(missing, "job")
			}
			if d.StepName == "" {
				missing = append(missing, "step_name")
			}
			if d.Owner == "" {
				missing = append(missing, "owner")
			}
			if d.Reason == "" {
				missing = append(missing, "reason")
			}
			if d.Expiry == "" {
				missing = append(missing, "expiry")
			}
			fmt.Fprintf(os.Stderr, "INVALID: deferral[%d] missing required field(s): %s\n",
				i, strings.Join(missing, ", "))
			failures++
			continue
		}

		// Duplicate detection
		key := d.File + "/" + d.Job + "/" + d.StepName
		if seen[key] {
			fmt.Fprintf(os.Stderr, "DUPLICATE: deferral[%d] %s is registered more than once\n", i, key)
			failures++
		}
		seen[key] = true

		// Expiry parse + not-past check
		expiry, err := time.Parse("2006-01-02", d.Expiry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: invalid expiry date %q for %s\n", d.Expiry, key)
			failures++
			continue
		}
		if expiry.Before(today) {
			fmt.Fprintf(os.Stderr, "EXPIRED: deferral %s expired on %s\n", key, d.Expiry)
			failures++
		}
		// ≤90 days cap
		if expiry.After(maxExpiry) {
			fmt.Fprintf(os.Stderr, "TOO_FAR: deferral %s expiry %s exceeds 90-day cap (%s)\n",
				key, d.Expiry, maxExpiry.Format("2006-01-02"))
			failures++
		}
	}

	// 3. Discover workflow files
	workflows, err := discoverWorkflows(workflowDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: cannot discover workflows: %v\n", err)
		os.Exit(1)
	}

	// 4. Check each workflow
	matchedDeferrals := make(map[string]bool)
	for _, wfPath := range workflows {
		wfFailures := checkWorkflowWithTracking(wfPath, deferrals, matchedDeferrals)
		failures += wfFailures
	}

	// 5. Stale deferral detection (entry exists but no matching run:| in workflows)
	for _, d := range deferrals.Deferrals {
		key := d.File + "/" + d.Job + "/" + d.StepName
		if !matchedDeferrals[key] {
			fmt.Fprintf(os.Stderr, "STALE: deferral %s does not match any run:| block in workflows\n", key)
			failures++
		}
	}

	// 6. Check Makefile parity (required targets)
	failures += checkMakefileParity(workflows)

	if failures > 0 {
		fmt.Fprintf(os.Stderr, "\nFAIL: workflow parity check found %d issue(s)\n", failures)
		os.Exit(1)
	}

	fmt.Println("OK: workflow parity check passed")
}

// ─── Core Checks ───────────────────────────────────────────────────────────

func checkWorkflow(path string, deferrals *DeferralTable) int {
	return checkWorkflowWithTracking(path, deferrals, nil)
}

func checkWorkflowWithTracking(path string, deferrals *DeferralTable, matchedDeferrals map[string]bool) int {
	failures := 0
	filename := filepath.Base(path)

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot read %s: %v\n", path, err)
		return 1
	}

	var wf WorkflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot parse %s: %v\n", path, err)
		return 1
	}

	// Rule: workflow-level permissions block must exist
	if wf.Permissions == nil {
		fmt.Fprintf(os.Stderr, "MISSING: %s has no workflow-level permissions: block\n", filename)
		failures++
	}

	// Rule: SHA + # vX.Y.Z pinning (check raw lines because YAML parser strips comments)
	failures += checkUsesPinRaw(path, filename, string(data))

	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			// Rule: unregistered run: | blocks
			if isMultilineRun(step.Run) {
				if isDeferredBlock(filename, jobName, step.Name, deferrals) {
					// Track matched deferral for stale detection
					if matchedDeferrals != nil {
						key := filename + "/" + jobName + "/" + step.Name
						matchedDeferrals[key] = true
					}
				} else {
					fmt.Fprintf(os.Stderr, "UNREGISTERED run:|: %s / job:%s / step:%q\n",
						filename, jobName, step.Name)
					fmt.Fprintf(os.Stderr, "  → Register in %s or convert to single-line `make` call\n",
						deferralPath)
					failures++
				}
			}
		}
	}

	return failures
}

// checkUsesPinRaw scans raw file content for `uses:` lines and validates
// the SHA + # vX.Y.Z pattern. We must use raw content because YAML parsers
// strip trailing comments (the `# vX.Y.Z` part).
func checkUsesPinRaw(path, filename, content string) int {
	failures := 0
	lines := strings.Split(content, "\n")

	// Matches: uses: owner/repo@<40-hex>
	usesLineRegex := regexp.MustCompile(`^\s*-?\s*uses:\s*(.+)$`)
	// Full valid pattern: owner/repo@<40-hex> # vX.Y.Z (with optional spaces)
	validPinRegex := regexp.MustCompile(`^[^/]+/[^@]+@[0-9a-f]{40}\s+#\s*v\d+\.\d+\.\d+`)

	for i, line := range lines {
		matches := usesLineRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		usesValue := strings.TrimSpace(matches[1])

		// Skip local actions and docker:// references
		if strings.HasPrefix(usesValue, "./") || strings.HasPrefix(usesValue, "docker://") {
			continue
		}

		if !validPinRegex.MatchString(usesValue) {
			fmt.Fprintf(os.Stderr, "PIN: %s:%d — uses: %q missing SHA + # vX.Y.Z\n",
				filename, i+1, usesValue)
			failures++
		}
	}
	return failures
}

func checkMakefileParity(workflows []string) int {
	failures := 0

	makefileBytes, err := os.ReadFile("Makefile")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot read Makefile: %v\n", err)
		return 1
	}
	makefile := string(makefileBytes)

	// Required make targets that must exist in workflow files
	type requirement struct {
		workflow string
		target   string
	}

	requirements := []requirement{
		{".github/workflows/ci.yml", "make secrets-scan"},
		{".github/workflows/ci.yml", "make govulncheck"},
		{".github/workflows/ci.yml", "make ci-go-lint"},
		{".github/workflows/ci.yml", "make ci-go-build"},
		{".github/workflows/ci.yml", "make ci-go-test"},
		{".github/workflows/ci.yml", "make ci-master-flow-backend"},
		{".github/workflows/ci.yml", "make ci-governance"},
		{".github/workflows/frontend-tests.yml", "make -C .. ci-frontend-deadcode"},
		{".github/workflows/frontend-tests.yml", "make -C .. ci-frontend-unit"},
		{".github/workflows/frontend-tests.yml", "make -C .. ci-e2e-smoke"},
		{".github/workflows/api-contract-validation.yml", "make ci-api-lint"},
		{".github/workflows/api-contract-validation.yml", "make ci-api-breaking"},
		{".github/workflows/api-contract-validation.yml", "make ci-api-generated-sync"},
		{".github/workflows/api-contract-validation.yml", "make ci-api-contract"},
	}

	// Check that required make calls exist in workflow files
	for _, req := range requirements {
		wfData, err := os.ReadFile(req.workflow)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: cannot read %s: %v\n", req.workflow, err)
			failures++
			continue
		}
		if !strings.Contains(string(wfData), req.target) {
			fmt.Fprintf(os.Stderr, "MISSING: %s does not contain %q\n", req.workflow, req.target)
			failures++
		}
	}

	// Forbidden patterns (direct tool invocations that should go through make)
	type forbidden struct {
		workflow string
		pattern  string
	}

	forbiddens := []forbidden{
		{".github/workflows/ci.yml", "uses: golangci/golangci-lint-action"},
		{".github/workflows/ci.yml", "run: go build ./..."},
		{".github/workflows/ci.yml", "run: go test -race -coverprofile=coverage.out -covermode=atomic ./..."},
		{".github/workflows/frontend-tests.yml", "run: npm audit --audit-level=high"},
		{".github/workflows/frontend-tests.yml", "run: npm run knip"},
		{".github/workflows/frontend-tests.yml", "run: npm run typecheck"},
		{".github/workflows/frontend-tests.yml", "run: npm run test:run"},
		{".github/workflows/frontend-tests.yml", "run: npm run build"},
		{".github/workflows/api-contract-validation.yml", "run: make api-compat-generate"},
		{".github/workflows/api-contract-validation.yml", "run: make api-generate"},
		{".github/workflows/api-contract-validation.yml", "run: make api-compat"},
		{".github/workflows/api-contract-validation.yml", "run: npm run typecheck"},
		{".github/workflows/api-contract-validation.yml", "run: go test ./internal/api/middleware/... -run OpenAPI -count=1"},
	}

	for _, f := range forbiddens {
		wfData, err := os.ReadFile(f.workflow)
		if err != nil {
			continue // already reported above
		}
		if strings.Contains(string(wfData), f.pattern) {
			fmt.Fprintf(os.Stderr, "FORBIDDEN: %s contains %q\n", f.workflow, f.pattern)
			failures++
		}
	}

	// Check required Makefile targets actually exist
	requiredTargets := []string{
		"ci-governance:",
		"ci-go-lint:",
		"ci-go-build:",
		"ci-go-test:",
		"ci-master-flow-backend:",
		"ci-frontend-deadcode:",
		"ci-frontend-unit:",
		"ci-e2e-smoke:",
		"ci-api-lint:",
		"ci-api-breaking:",
		"ci-api-generated-sync:",
		"ci-api-contract:",
		"secrets-scan:",
		"govulncheck:",
		"dco-check:",
		"api-changelog-comment:",
	}

	for _, target := range requiredTargets {
		if !strings.Contains(makefile, target) {
			fmt.Fprintf(os.Stderr, "MISSING TARGET: Makefile does not define %s\n", target)
			failures++
		}
	}

	return failures
}

// ─── Fixture Testing Mode ──────────────────────────────────────────────────

func checkFixture(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read fixture %s: %w", path, err)
	}

	var wf WorkflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return fmt.Errorf("cannot parse fixture %s: %w", path, err)
	}

	// Load deferral table for cross-reference
	deferrals, err := loadDeferralTable(deferralPath)
	if err != nil {
		// In fixture mode, missing deferral table means empty
		deferrals = &DeferralTable{}
	}

	filename := filepath.Base(path)
	failures := 0

	// SHA + # vX.Y.Z check on raw content
	failures += checkUsesPinRaw(path, filename, string(data))

	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			if isMultilineRun(step.Run) {
				// In fixture mode, match on job+step_name only (ignore filename)
				// so fixtures can test deferral logic without needing to match
				// real workflow filenames.
				if !isDeferredBlockLoose(jobName, step.Name, deferrals) {
					fmt.Fprintf(os.Stderr, "UNREGISTERED run:|: %s / job:%s / step:%q\n",
						filename, jobName, step.Name)
					failures++
				}
			}
		}
	}

	if failures > 0 {
		return fmt.Errorf("fixture %s has %d violation(s)", filepath.Base(path), failures)
	}
	return nil
}

// ─── Helpers ───────────────────────────────────────────────────────────────

func loadDeferralTable(path string) (*DeferralTable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var table DeferralTable
	if err := yaml.Unmarshal(data, &table); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &table, nil
}

func discoverWorkflows(dir string) ([]string, error) {
	var paths []string
	for _, ext := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(dir, ext))
		if err != nil {
			return nil, err
		}
		paths = append(paths, matches...)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no workflow files found in %s", dir)
	}
	return paths, nil
}

func isMultilineRun(run string) bool {
	return strings.Contains(run, "\n")
}

func isDeferredBlock(filename, jobName, stepName string, deferrals *DeferralTable) bool {
	for _, d := range deferrals.Deferrals {
		if d.File == filename && d.Job == jobName && d.StepName == stepName {
			return true
		}
	}
	return false
}

// isDeferredBlockLoose matches on job+step_name only (used in fixture mode
// where the file name doesn't correspond to a real workflow).
func isDeferredBlockLoose(jobName, stepName string, deferrals *DeferralTable) bool {
	for _, d := range deferrals.Deferrals {
		if d.Job == jobName && d.StepName == stepName {
			return true
		}
	}
	return false
}
