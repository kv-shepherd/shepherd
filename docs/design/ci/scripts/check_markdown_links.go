package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type linkRef struct {
	source string
	line   int
	target string
}

type failure struct {
	source string
	line   int
	target string
	reason string
}

var (
	inlineLinkRe = regexp.MustCompile(`!?\[[^\]]+\]\(([^)]+)\)`)
	refLinkRe    = regexp.MustCompile(`^\s*\[[^\]]+\]:\s*(\S+)`)
	headingRe    = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	explicitIDRe = regexp.MustCompile(`\{#([^}]+)\}`)
	htmlIDRe     = regexp.MustCompile(`<a\s+id=["']([^"']+)["']\s*></a>`)
	mdLinkTextRe = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	htmlTagRe    = regexp.MustCompile(`<[^>]+>`)
)

func main() {
	roots := os.Args[1:]
	explicitRoots := len(roots) > 0
	if !explicitRoots {
		roots = []string{"docs/design", "docs/i18n/zh-CN/design", "docs/adr"}
	}

	var files []string
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			if explicitRoots {
				fmt.Fprintf(os.Stderr, "[markdown-links] root not found: %s\n", root)
				os.Exit(1)
			}
			continue
		}
		if !info.IsDir() {
			cleaned := filepath.Clean(root)
			if strings.HasSuffix(strings.ToLower(cleaned), ".md") {
				files = append(files, cleaned)
			}
			continue
		}

		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(path), ".md") {
				files = append(files, filepath.Clean(path))
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "walk failed for %s: %v\n", root, err)
			os.Exit(1)
		}
	}
	if len(files) == 0 {
		if explicitRoots {
			fmt.Fprintln(os.Stderr, "[markdown-links] no markdown files found from explicit roots")
			os.Exit(1)
		}
		fmt.Println("[markdown-links] no markdown files found")
		return
	}
	sort.Strings(files)

	anchorCache := make(map[string]map[string]struct{})
	var failures []failure

	for _, file := range files {
		links, err := extractLinks(file)
		if err != nil {
			failures = append(failures, failure{source: file, line: 1, target: "", reason: "read failed: " + err.Error()})
			continue
		}

		for _, link := range links {
			ok, reason := validateLink(link, anchorCache)
			if !ok {
				failures = append(failures, failure{
					source: link.source,
					line:   link.line,
					target: link.target,
					reason: reason,
				})
			}
		}
	}

	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "%s:%d: broken link %q (%s)\n", f.source, f.line, f.target, f.reason)
		}
		fmt.Fprintf(os.Stderr, "[markdown-links] FAIL: %d broken references\n", len(failures))
		os.Exit(1)
	}

	fmt.Printf("[markdown-links] OK: %d markdown files checked\n", len(files))
}

func extractLinks(file string) ([]linkRef, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var links []linkRef
	s := bufio.NewScanner(f)
	lineNo := 0
	inFence := false

	for s.Scan() {
		lineNo++
		line := s.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		for _, m := range inlineLinkRe.FindAllStringSubmatch(line, -1) {
			if len(m) < 2 {
				continue
			}
			target := normalizeTarget(m[1])
			if target == "" {
				continue
			}
			links = append(links, linkRef{source: file, line: lineNo, target: target})
		}

		if m := refLinkRe.FindStringSubmatch(line); len(m) == 2 {
			target := normalizeTarget(m[1])
			if target != "" {
				links = append(links, linkRef{source: file, line: lineNo, target: target})
			}
		}
	}

	if err := s.Err(); err != nil {
		return nil, err
	}
	return links, nil
}

func normalizeTarget(raw string) string {
	t := strings.TrimSpace(raw)
	if t == "" {
		return ""
	}
	if strings.HasPrefix(t, "<") && strings.Contains(t, ">") {
		t = strings.TrimPrefix(t, "<")
		t = t[:strings.Index(t, ">")]
		return strings.TrimSpace(t)
	}
	if fields := strings.Fields(t); len(fields) > 0 {
		t = fields[0]
	}
	t = strings.Trim(t, "\"'")
	return t
}

func validateLink(link linkRef, cache map[string]map[string]struct{}) (bool, string) {
	target := link.target
	if isExternal(target) {
		return true, ""
	}

	var targetFile string
	var anchor string

	if strings.HasPrefix(target, "#") {
		targetFile = link.source
		anchor = strings.TrimPrefix(target, "#")
	} else {
		parts := strings.SplitN(target, "#", 2)
		pathPart := decode(parts[0])
		if pathPart == "" {
			pathPart = "."
		}
		if filepath.IsAbs(pathPart) {
			targetFile = filepath.Clean(pathPart)
		} else {
			targetFile = filepath.Clean(filepath.Join(filepath.Dir(link.source), pathPart))
		}
		if len(parts) == 2 {
			anchor = parts[1]
		}
	}

	info, err := os.Stat(targetFile)
	if err != nil {
		return false, "target file does not exist"
	}
	if info.IsDir() {
		indexFile := filepath.Join(targetFile, "README.md")
		if _, err := os.Stat(indexFile); err != nil {
			return false, "target resolves to directory without README.md"
		}
		targetFile = indexFile
	}

	if anchor == "" {
		return true, ""
	}
	anchor = strings.TrimSpace(decode(anchor))
	if anchor == "" {
		return true, ""
	}

	anchors, ok := cache[targetFile]
	if !ok {
		var err error
		anchors, err = collectAnchors(targetFile)
		if err != nil {
			return false, "cannot parse anchors: " + err.Error()
		}
		cache[targetFile] = anchors
	}

	if _, exists := anchors[anchor]; exists {
		return true, ""
	}
	if _, exists := anchors[strings.ToLower(anchor)]; exists {
		return true, ""
	}

	return false, "anchor not found"
}

func isExternal(link string) bool {
	lower := strings.ToLower(link)
	for _, p := range []string{"http://", "https://", "mailto:", "tel:", "data:", "javascript:"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func decode(s string) string {
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return decoded
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
