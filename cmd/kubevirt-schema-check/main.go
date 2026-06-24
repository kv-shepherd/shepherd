package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type manifest struct {
	Entities map[string]struct {
		CurrentVersion string `json:"current_version"`
	} `json:"entities"`
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

const (
	manifestPath                    = "internal/pkg/schema/manifest.json"
	ghReleasesURL                   = "https://api.github.com/repos/kubevirt/kubevirt/releases?per_page=30"
	schemaMaintenanceRequestTimeout = 30 * time.Second
	githubReleasesMaxResponseBytes  = int64(2 * 1024 * 1024)
)

var gaTagRe = regexp.MustCompile(`^v(\d+\.\d+\.\d+)$`)

func main() {
	localVersion, err := readLocalVersionFromManifest(manifestPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Local embedded schema version:  v%s\n", localVersion)

	latestGA, err := fetchLatestGA()
	if err != nil {
		fmt.Printf("WARNING: cannot determine upstream version — skipping (%v)\n", err)
		os.Exit(0)
	}
	fmt.Printf("Latest KubeVirt GA release:     v%s\n", latestGA)

	cmp := compareSemver(localVersion, latestGA)
	if cmp >= 0 {
		fmt.Printf("✅ Embedded schema is up-to-date (v%s >= v%s)\n", localVersion, latestGA)
		os.Exit(0)
	}

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("⚠️  SCHEMA VERSION DRIFT DETECTED")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("  Embedded:  v%s\n", localVersion)
	fmt.Printf("  Upstream:  v%s\n", latestGA)
	fmt.Println()
	fmt.Println("  Action required:")
	for index, step := range driftActionSteps(latestGA) {
		fmt.Printf("    %d. %s\n", index+1, step)
	}
	fmt.Println()
	fmt.Println("  See: docs/design/DEPENDENCIES.md for version alignment guide")
	fmt.Println("============================================================")

	if os.Getenv("KUBEVIRT_SCHEMA_FRESHNESS_BLOCKING") == "true" {
		os.Exit(1)
	}
}

func readLocalVersionFromManifest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	for _, entity := range m.Entities {
		v := strings.TrimPrefix(entity.CurrentVersion, "kubevirt-v")
		if v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("no current_version found in %s", path)
}

func fetchLatestGA() (string, error) {
	return fetchLatestGAFrom(http.DefaultClient, ghReleasesURL)
}

func fetchLatestGAFrom(client *http.Client, releasesURL string) (string, error) {
	return fetchLatestGAFromWithLimit(client, releasesURL, githubReleasesMaxResponseBytes)
}

func fetchLatestGAFromWithLimit(client *http.Client, releasesURL string, maxResponseBytes int64) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), schemaMaintenanceRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	// #nosec G704 -- fixed GitHub releases API endpoint for advisory freshness check.
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 201))
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body[:minInt(len(body), 200)]))
	}

	payload, err := readLimitedResponseBody(resp.Body, maxResponseBytes, "GitHub releases response")
	if err != nil {
		return "", err
	}
	var releases []ghRelease
	if err := json.Unmarshal(payload, &releases); err != nil {
		return "", fmt.Errorf("decode releases: %w", err)
	}

	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		m := gaTagRe.FindStringSubmatch(r.TagName)
		if m == nil {
			continue
		}
		return m[1], nil
	}
	return "", fmt.Errorf("no GA release found in latest %d releases", len(releases))
}

func readLimitedResponseBody(r io.Reader, maxBytes int64, label string) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("%s size limit must be positive", label)
	}
	payload, err := io.ReadAll(&io.LimitedReader{R: r, N: maxBytes + 1})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	if int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, maxBytes)
	}
	return payload, nil
}

func compareSemver(a, b string) int {
	ap := parseSemver(a)
	bp := parseSemver(b)
	for i := 0; i < 3; i++ {
		if ap[i] < bp[i] {
			return -1
		}
		if ap[i] > bp[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(s string) [3]int {
	parts := strings.SplitN(s, ".", 3)
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		result[i] = n
	}
	return result
}

func driftActionSteps(latestGA string) []string {
	return []string{
		fmt.Sprintf("Run:  make kubevirt-schema-upgrade VERSION=%s", latestGA),
		"Review diff in internal/pkg/schema/versions/",
		"Run:  make kubevirt-schema-report",
		"Update instancesize mask/i18n only for fields you choose to expose",
		"Update go.mod: kubevirt.io/api + kubevirt.io/client-go",
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
