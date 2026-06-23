package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCompareSemver(t *testing.T) {
	t.Parallel()

	if got := compareSemver("1.7.0", "1.8.0"); got >= 0 {
		t.Fatalf("compareSemver(1.7.0, 1.8.0) = %d, want < 0", got)
	}
	if got := compareSemver("1.8.0", "1.8.0"); got != 0 {
		t.Fatalf("compareSemver(1.8.0, 1.8.0) = %d, want 0", got)
	}
	if got := compareSemver("1.8.1", "1.8.0"); got <= 0 {
		t.Fatalf("compareSemver(1.8.1, 1.8.0) = %d, want > 0", got)
	}
}

func TestReadLocalVersionFromManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	data := []byte(`{"entities":{"instancesize":{"current_version":"kubevirt-v1.7.0"}}}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := readLocalVersionFromManifest(path)
	if err != nil {
		t.Fatalf("readLocalVersionFromManifest returned error: %v", err)
	}
	if got != "1.7.0" {
		t.Fatalf("readLocalVersionFromManifest = %q, want %q", got, "1.7.0")
	}
}

func TestReadLocalVersionFromManifestErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "invalid json",
			data: `{`,
			want: "parse ",
		},
		{
			name: "missing current version",
			data: `{"entities":{"instancesize":{"current_version":""}}}`,
			want: "no current_version found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "manifest.json")
			if err := os.WriteFile(path, []byte(tc.data), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			_, err := readLocalVersionFromManifest(path)
			if err == nil {
				t.Fatal("readLocalVersionFromManifest() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("readLocalVersionFromManifest() error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestFetchLatestGAFromFiltersDraftPrereleaseAndInvalidTags(t *testing.T) {
	var sawAccept bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAccept = r.Header.Get("Accept") == "application/vnd.github+json"
		fmt.Fprint(w, `[
			{"tag_name":"v2.0.0-alpha.1","draft":false,"prerelease":false},
			{"tag_name":"v1.9.0","draft":true,"prerelease":false},
			{"tag_name":"v1.8.5","draft":false,"prerelease":true},
			{"tag_name":"v1.8.4","draft":false,"prerelease":false}
		]`)
	}))
	defer server.Close()

	got, err := fetchLatestGAFrom(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetchLatestGAFrom() error = %v", err)
	}
	if got != "1.8.4" {
		t.Fatalf("fetchLatestGAFrom() = %q, want %q", got, "1.8.4")
	}
	if !sawAccept {
		t.Fatal("fetchLatestGAFrom() did not send GitHub JSON Accept header")
	}
}

func TestFetchLatestGAFromUsesGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token-value")

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		fmt.Fprint(w, `[{"tag_name":"v1.8.4","draft":false,"prerelease":false}]`)
	}))
	defer server.Close()

	if _, err := fetchLatestGAFrom(server.Client(), server.URL); err != nil {
		t.Fatalf("fetchLatestGAFrom() error = %v", err)
	}
	if authHeader != "token token-value" {
		t.Fatalf("Authorization header = %q, want %q", authHeader, "token token-value")
	}
}

func TestFetchLatestGAFromUsesBoundedRequestContext(t *testing.T) {
	var sawDeadline bool
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok {
			t.Fatal("request context missing deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > schemaMaintenanceRequestTimeout {
			t.Fatalf("request context deadline remaining = %s, want within %s", remaining, schemaMaintenanceRequestTimeout)
		}
		sawDeadline = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`[{"tag_name":"v1.8.4","draft":false,"prerelease":false}]`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	got, err := fetchLatestGAFrom(client, "https://example.test/releases")
	if err != nil {
		t.Fatalf("fetchLatestGAFrom() error = %v", err)
	}
	if got != "1.8.4" {
		t.Fatalf("fetchLatestGAFrom() = %q, want %q", got, "1.8.4")
	}
	if !sawDeadline {
		t.Fatal("fetchLatestGAFrom() did not execute test transport")
	}
}

func TestFetchLatestGAFromRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"tag_name":"v1.8.4","draft":false,"prerelease":false}]`)
	}))
	defer server.Close()

	_, err := fetchLatestGAFromWithLimit(server.Client(), server.URL, 32)
	if err == nil {
		t.Fatal("fetchLatestGAFromWithLimit() error = nil, want oversized response error")
	}
	if !strings.Contains(err.Error(), "GitHub releases response exceeds 32 bytes") {
		t.Fatalf("fetchLatestGAFromWithLimit() error = %q, want oversized response error", err.Error())
	}
}

func TestFetchLatestGAFromErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "non ok response truncates body",
			status: http.StatusForbidden,
			body:   strings.Repeat("x", 250),
			want:   "GitHub API returned 403: " + strings.Repeat("x", 200),
		},
		{
			name:   "invalid json",
			status: http.StatusOK,
			body:   `{`,
			want:   "decode releases",
		},
		{
			name:   "no ga release",
			status: http.StatusOK,
			body:   `[{"tag_name":"v1.8.4","draft":false,"prerelease":true}]`,
			want:   "no GA release found in latest 1 releases",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()

			_, err := fetchLatestGAFrom(server.Client(), server.URL)
			if err == nil {
				t.Fatal("fetchLatestGAFrom() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("fetchLatestGAFrom() error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestDriftActionSteps(t *testing.T) {
	t.Parallel()

	got := driftActionSteps("1.8.0")
	want := []string{
		"Run:  make kubevirt-schema-upgrade VERSION=1.8.0",
		"Review diff in internal/pkg/schema/versions/",
		"Run:  make kubevirt-schema-report",
		"Update instancesize mask/i18n only for fields you choose to expose",
		"Update go.mod: kubevirt.io/api + kubevirt.io/client-go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("driftActionSteps() = %#v, want %#v", got, want)
	}
}

func TestMinInt(t *testing.T) {
	t.Parallel()

	if got := minInt(3, 5); got != 3 {
		t.Fatalf("minInt(3, 5) = %d, want 3", got)
	}
	if got := minInt(7, 2); got != 2 {
		t.Fatalf("minInt(7, 2) = %d, want 2", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
