package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type manifest struct {
	Version    int          `json:"version"`
	MasterFlow string       `json:"master_flow"`
	Stages     []stageEntry `json:"stages"`
}

type stageEntry struct {
	ID         string   `json:"id"`
	Phases     []string `json:"phases"`
	Checklists []string `json:"checklists,omitempty"`
	CI         []string `json:"ci,omitempty"`
	ADRs       []string `json:"adrs,omitempty"`
	Examples   []string `json:"examples,omitempty"`
}

var (
	headingRe      = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	explicitIDRe   = regexp.MustCompile(`\{#([^}]+)\}`)
	htmlIDRe       = regexp.MustCompile(`<a\s+id=["']([^"']+)["']\s*></a>`)
	mdLinkTextRe   = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	htmlTagRe      = regexp.MustCompile(`<[^>]+>`)
	stageHeadingRe = regexp.MustCompile(`(?i)^stage\s+[0-9]`)
)

func main() {
	manifestPath := flag.String("manifest", "docs/design/traceability/master-flow.json", "path to traceability manifest")
	flag.Parse()

	m, err := readManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[traceability] FAIL: %v\n", err)
		os.Exit(1)
	}
	if m.Version != 1 {
		fmt.Fprintf(os.Stderr, "[traceability] FAIL: unsupported manifest version: %d\n", m.Version)
		os.Exit(1)
	}
	if strings.TrimSpace(m.MasterFlow) == "" {
		fmt.Fprintln(os.Stderr, "[traceability] FAIL: manifest.master_flow must be set")
		os.Exit(1)
	}

	masterStages, err := collectMasterFlowStages(m.MasterFlow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[traceability] FAIL: collect master-flow stages: %v\n", err)
		os.Exit(1)
	}

	stageByID := make(map[string]stageEntry, len(m.Stages))
	var problems []string

	for _, s := range m.Stages {
		id := strings.TrimSpace(s.ID)
		if id == "" {
			problems = append(problems, "manifest contains stage with empty id")
			continue
		}
		if _, exists := stageByID[id]; exists {
			problems = append(problems, fmt.Sprintf("duplicate stage id in manifest: %q", id))
			continue
		}
		stageByID[id] = s

		if len(s.Phases) == 0 {
			problems = append(problems, fmt.Sprintf("stage %q: phases must be non-empty", id))
		}
		if len(s.Checklists) == 0 && len(s.CI) == 0 {
			problems = append(problems, fmt.Sprintf("stage %q: must include at least one checklist or ci gate", id))
		}

		problems = append(problems, validateRefs(id, "phases", s.Phases)...)
		problems = append(problems, validateRefs(id, "checklists", s.Checklists)...)
		problems = append(problems, validateRefs(id, "ci", s.CI)...)
		problems = append(problems, validateRefs(id, "adrs", s.ADRs)...)
		problems = append(problems, validateRefs(id, "examples", s.Examples)...)
	}

	for id := range masterStages {
		if _, ok := stageByID[id]; !ok {
			problems = append(problems, fmt.Sprintf("missing stage mapping for master-flow id: %q", id))
		}
	}
	for id := range stageByID {
		if _, ok := masterStages[id]; !ok {
			problems = append(problems, fmt.Sprintf("unknown stage id in manifest (not found in master-flow): %q", id))
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "[traceability] %s\n", p)
		}
		fmt.Fprintf(os.Stderr, "[traceability] FAIL: %d problem(s)\n", len(problems))
		os.Exit(1)
	}

	fmt.Printf("[traceability] OK: %d stages mapped\n", len(masterStages))
}

func readManifest(path string) (*manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var m manifest
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return &m, nil
}

func collectMasterFlowStages(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stages := make(map[string]struct{})
	s := bufio.NewScanner(f)
	inFence := false

	for s.Scan() {
		line := s.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		for _, m := range htmlIDRe.FindAllStringSubmatch(line, -1) {
			if len(m) == 2 {
				id := strings.TrimSpace(m[1])
				if strings.HasPrefix(id, "stage-") {
					stages[id] = struct{}{}
				}
			}
		}

		h := headingRe.FindStringSubmatch(line)
		if len(h) != 2 {
			continue
		}
		heading := strings.TrimSpace(h[1])

		var explicitStageIDs []string
		for _, m := range explicitIDRe.FindAllStringSubmatch(heading, -1) {
			if len(m) != 2 {
				continue
			}
			id := strings.TrimSpace(m[1])
			if strings.HasPrefix(id, "stage-") {
				explicitStageIDs = append(explicitStageIDs, id)
				stages[id] = struct{}{}
			}
		}

		headingNoID := strings.TrimSpace(explicitIDRe.ReplaceAllString(heading, ""))
		if len(explicitStageIDs) > 0 {
			// Prefer explicit IDs as canonical stage identifiers.
			continue
		}
		headingNoID = mdLinkTextRe.ReplaceAllString(headingNoID, "$1")
		headingNoID = strings.ReplaceAll(headingNoID, "`", "")
		headingNoID = htmlTagRe.ReplaceAllString(headingNoID, "")
		if !stageHeadingRe.MatchString(headingNoID) {
			continue
		}
		// Only treat headings with ":" as canonical "Stage" sections to avoid
		// pulling in note subsections like "Stage 2 ... Notes".
		if !strings.Contains(headingNoID, ":") {
			continue
		}
		slug := githubSlug(headingNoID)
		if strings.HasPrefix(slug, "stage-") {
			stages[slug] = struct{}{}
		}
	}

	if err := s.Err(); err != nil {
		return nil, err
	}
	return stages, nil
}

func validateRefs(stageID, field string, refs []string) []string {
	var problems []string
	for i, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			problems = append(problems, fmt.Sprintf("stage %q: %s[%d] is empty", stageID, field, i))
			continue
		}
		pathPart, anchor := splitRef(ref)
		if pathPart == "" {
			problems = append(problems, fmt.Sprintf("stage %q: %s[%d] invalid ref %q", stageID, field, i, ref))
			continue
		}
		if err := validatePathAnchor(pathPart, anchor); err != nil {
			problems = append(problems, fmt.Sprintf("stage %q: %s[%d] %q: %v", stageID, field, i, ref, err))
		}
	}
	return problems
}

func splitRef(ref string) (string, string) {
	parts := strings.SplitN(ref, "#", 2)
	pathPart := strings.TrimSpace(parts[0])
	var anchor string
	if len(parts) == 2 {
		anchor = strings.TrimSpace(parts[1])
	}
	return pathPart, anchor
}

func validatePathAnchor(pathPart, anchor string) error {
	clean := filepath.Clean(pathPart)
	info, err := os.Stat(clean)
	if err != nil {
		return fmt.Errorf("target does not exist")
	}
	if info.IsDir() {
		return fmt.Errorf("target resolves to directory")
	}

	if anchor == "" {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(clean))
	if ext != ".md" {
		return fmt.Errorf("anchor specified for non-markdown target")
	}

	anchors, err := collectAnchors(clean)
	if err != nil {
		return fmt.Errorf("cannot parse anchors: %w", err)
	}
	if _, ok := anchors[anchor]; ok {
		return nil
	}
	if _, ok := anchors[strings.ToLower(anchor)]; ok {
		return nil
	}
	return fmt.Errorf("anchor not found")
}

func collectAnchors(file string) (map[string]struct{}, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	anchors := make(map[string]struct{})
	slugCount := make(map[string]int)
	s := bufio.NewScanner(f)
	inFence := false

	for s.Scan() {
		line := s.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		for _, m := range htmlIDRe.FindAllStringSubmatch(line, -1) {
			if len(m) == 2 && strings.TrimSpace(m[1]) != "" {
				anchors[m[1]] = struct{}{}
				anchors[strings.ToLower(m[1])] = struct{}{}
			}
		}

		h := headingRe.FindStringSubmatch(line)
		if len(h) != 2 {
			continue
		}
		heading := strings.TrimSpace(h[1])

		for _, m := range explicitIDRe.FindAllStringSubmatch(heading, -1) {
			if len(m) == 2 && strings.TrimSpace(m[1]) != "" {
				anchors[m[1]] = struct{}{}
				anchors[strings.ToLower(m[1])] = struct{}{}
			}
		}

		heading = explicitIDRe.ReplaceAllString(heading, "")
		heading = strings.TrimSpace(heading)
		if heading == "" {
			continue
		}

		heading = mdLinkTextRe.ReplaceAllString(heading, "$1")
		heading = strings.ReplaceAll(heading, "`", "")
		heading = htmlTagRe.ReplaceAllString(heading, "")

		slug := githubSlug(heading)
		if slug == "" {
			continue
		}
		count := slugCount[slug]
		if count == 0 {
			anchors[slug] = struct{}{}
		} else {
			anchors[fmt.Sprintf("%s-%d", slug, count)] = struct{}{}
		}
		slugCount[slug] = count + 1
	}

	if err := s.Err(); err != nil {
		return nil, err
	}
	return anchors, nil
}

func githubSlug(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}

	var b strings.Builder
	lastHyphen := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastHyphen = false
		case r == '-' || unicode.IsSpace(r):
			if !lastHyphen {
				b.WriteRune('-')
				lastHyphen = true
			}
		case r == '_':
			b.WriteRune('_')
			lastHyphen = false
		default:
			// drop punctuation and symbols
		}
	}
	return strings.Trim(b.String(), "-")
}
