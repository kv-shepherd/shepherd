package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type rewriteWeComTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t *rewriteWeComTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

func newWeComTestHTTPClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	base := server.Client().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Transport: &rewriteWeComTransport{target: target, base: base},
		Timeout:   wecomDefaultRequestTimeout,
	}
}

func TestWeComStartLogin_BuildsQRCodeRedirect(t *testing.T) {
	t.Parallel()

	adapter := newWeComAuthProviderAdapter()
	resp, err := adapter.StartLogin(t.Context(), map[string]interface{}{
		"corp_id":      "wwcorp",
		"agent_id":     "1000002",
		"agent_secret": "secret",
	}, AuthStartRequest{
		LoginMode:   wecomLoginModeQR,
		CallbackURL: "https://api.example.com/api/v1/auth/providers/p-1/callback",
		State:       "state-1",
	})
	if err != nil {
		t.Fatalf("StartLogin() error = %v", err)
	}

	redirectURL, err := url.Parse(resp.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if redirectURL.Host != "open.work.weixin.qq.com" {
		t.Fatalf("redirect host = %q, want %q", redirectURL.Host, "open.work.weixin.qq.com")
	}
	if redirectURL.Path != "/wwopen/sso/qrConnect" {
		t.Fatalf("redirect path = %q, want %q", redirectURL.Path, "/wwopen/sso/qrConnect")
	}
	query := redirectURL.Query()
	if query.Get("appid") != "wwcorp" {
		t.Fatalf("appid = %q, want %q", query.Get("appid"), "wwcorp")
	}
	if query.Get("agentid") != "1000002" {
		t.Fatalf("agentid = %q, want %q", query.Get("agentid"), "1000002")
	}
	if query.Get("state") != "state-1" {
		t.Fatalf("state = %q, want %q", query.Get("state"), "state-1")
	}
}

func TestWeComStartLogin_RejectsInWeComModeOutsideEmbeddedClient(t *testing.T) {
	t.Parallel()

	adapter := newWeComAuthProviderAdapter()
	_, err := adapter.StartLogin(t.Context(), map[string]interface{}{
		"corp_id":      "wwcorp",
		"agent_id":     "1000002",
		"agent_secret": "secret",
	}, AuthStartRequest{
		LoginMode:   wecomLoginModeInWeCom,
		CallbackURL: "https://console.example.com/api/v1/auth/providers/p-1/callback",
		State:       "state-1",
		UserAgent:   "Mozilla/5.0",
	})
	if err == nil {
		t.Fatal("StartLogin() error = nil, want auth start error")
	}
	var startErr *AuthStartError
	if !errors.As(err, &startErr) {
		t.Fatalf("StartLogin() error type = %T, want *AuthStartError", err)
	}
	if startErr.Code != "AUTH_LOGIN_MODE_UNAVAILABLE" {
		t.Fatalf("error code = %q, want %q", startErr.Code, "AUTH_LOGIN_MODE_UNAVAILABLE")
	}
}

func TestWeComCompleteLogin_MapsCanonicalAuthResult(t *testing.T) {
	t.Parallel()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/gettoken"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errcode":      0,
				"errmsg":       "ok",
				"access_token": "access-token-1",
			})
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/auth/getuserinfo"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errcode": 0,
				"errmsg":  "ok",
				"UserId":  "alice",
			})
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/user/get"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errcode":      0,
				"errmsg":       "ok",
				"userid":       "alice",
				"name":         "Alice Zhang",
				"alias":        "Alice",
				"mobile":       "13800000000",
				"email":        "alice@example.com",
				"position":     "SRE",
				"avatar":       "https://img.example.com/avatar.png",
				"english_name": "alice.zhang",
				"status":       1,
				"department":   []int{2},
			})
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/department/list"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errcode": 0,
				"errmsg":  "ok",
				"department": []map[string]interface{}{
					{"id": 2, "name": "Engineering"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	adapter := &wecomAuthProviderAdapter{
		openBaseURL:  defaultWeComOpenBaseURL,
		oauthBaseURL: defaultWeComOAuthBaseURL,
		httpClient:   newWeComTestHTTPClient(t, apiServer),
	}

	result, err := adapter.CompleteLogin(context.Background(), map[string]interface{}{
		"corp_id":      "wwcorp",
		"agent_id":     "1000002",
		"agent_secret": "secret",
	}, AuthCallbackRequest{
		Query: map[string][]string{
			"code": {"code-1"},
		},
	})
	if err != nil {
		t.Fatalf("CompleteLogin() error = %v", err)
	}
	if result.ExternalID != "alice" {
		t.Fatalf("external_id = %q, want %q", result.ExternalID, "alice")
	}
	if result.Username != "alice.zhang" {
		t.Fatalf("username = %q, want %q", result.Username, "alice.zhang")
	}
	if result.DisplayName != "Alice Zhang" {
		t.Fatalf("display_name = %q, want %q", result.DisplayName, "Alice Zhang")
	}
	if result.Email != "alice@example.com" {
		t.Fatalf("email = %q, want %q", result.Email, "alice@example.com")
	}
	if len(result.Cohorts) != 1 || result.Cohorts[0].DisplayName != "Engineering" {
		t.Fatalf("cohorts = %#v, want engineering department", result.Cohorts)
	}
	if result.Cohorts[0].Key != "2" {
		t.Fatalf("cohort key = %q, want %q", result.Cohorts[0].Key, "2")
	}
	if got := result.ProfileAttributes["phone_number"]; got != "13800000000" {
		t.Fatalf("phone_number = %#v, want %q", got, "13800000000")
	}
}

func TestWeComTestConnection_UsesGetToken(t *testing.T) {
	t.Parallel()

	var called bool
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/cgi-bin/gettoken") {
			called = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errcode":      0,
				"errmsg":       "ok",
				"access_token": "access-token-1",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer apiServer.Close()

	adapter := &wecomAuthProviderAdapter{
		openBaseURL:  defaultWeComOpenBaseURL,
		oauthBaseURL: defaultWeComOAuthBaseURL,
		httpClient:   newWeComTestHTTPClient(t, apiServer),
	}

	ok, message, err := adapter.TestConnection(t.Context(), map[string]interface{}{
		"corp_id":      "wwcorp",
		"agent_id":     "1000002",
		"agent_secret": "secret",
	})
	if err != nil {
		t.Fatalf("TestConnection() error = %v", err)
	}
	if !ok {
		t.Fatalf("TestConnection() success = false, message = %q", message)
	}
	if !called {
		t.Fatal("gettoken endpoint was not called")
	}
}

func TestWeComSupportsDirectorySyncCapability(t *testing.T) {
	t.Parallel()

	adapter := newWeComAuthProviderAdapter()
	if _, ok := interface{}(adapter).(DirectorySyncCapability); !ok {
		t.Fatal("wecom adapter does not implement DirectorySyncCapability")
	}
	if _, ok := interface{}(adapter).(ScheduledDirectoryEnrichmentCapability); !ok {
		t.Fatal("wecom adapter does not implement ScheduledDirectoryEnrichmentCapability")
	}
}

func TestWeComListDirectoryUsers_MapsDepartmentCohorts(t *testing.T) {
	t.Parallel()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/gettoken"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errcode":      0,
				"errmsg":       "ok",
				"access_token": "access-token-1",
			})
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/user/list"):
			if got := r.URL.Query().Get("department_id"); got != "2" {
				t.Fatalf("department_id = %q, want %q", got, "2")
			}
			if got := r.URL.Query().Get("fetch_child"); got != "1" {
				t.Fatalf("fetch_child = %q, want %q", got, "1")
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errcode": 0,
				"errmsg":  "ok",
				"userlist": []map[string]interface{}{
					{
						"userid":       "alice",
						"name":         "Alice Zhang",
						"alias":        "Alice",
						"mobile":       "13800000000",
						"email":        "alice@example.com",
						"position":     "SRE",
						"avatar":       "https://img.example.com/avatar.png",
						"english_name": "alice.zhang",
						"status":       1,
						"department":   []int{2, 3},
					},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/department/list"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"errcode": 0,
				"errmsg":  "ok",
				"department": []map[string]interface{}{
					{"id": 2, "name": "Engineering"},
					{"id": 3, "name": "Platform"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	adapter := &wecomAuthProviderAdapter{
		openBaseURL:  defaultWeComOpenBaseURL,
		oauthBaseURL: defaultWeComOAuthBaseURL,
		httpClient:   newWeComTestHTTPClient(t, apiServer),
	}

	records, err := adapter.ListDirectoryUsers(t.Context(), map[string]interface{}{
		"corp_id":      "wwcorp",
		"agent_id":     "1000002",
		"agent_secret": "secret",
	}, map[string]interface{}{
		"department_ids": []interface{}{"2"},
		"include_nested": true,
	})
	if err != nil {
		t.Fatalf("ListDirectoryUsers() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	record := records[0]
	if record.Username != "alice.zhang" {
		t.Fatalf("username = %q, want %q", record.Username, "alice.zhang")
	}
	if len(record.Cohorts) != 2 {
		t.Fatalf("cohorts = %#v, want 2 departments", record.Cohorts)
	}
	if record.Cohorts[0].Kind != "department" || record.Cohorts[0].Key != "2" {
		t.Fatalf("first cohort = %#v, want department 2", record.Cohorts[0])
	}
	if got := record.Attributes["organization_unit"]; got == nil {
		t.Fatalf("organization_unit = %#v, want populated departments", got)
	}
}

func TestWeComBuildsScheduledDirectoryEnrichmentPlan(t *testing.T) {
	t.Parallel()

	adapter := newWeComAuthProviderAdapter()
	plan, err := adapter.BuildScheduledDirectoryEnrichmentPlan(t.Context(), map[string]interface{}{
		"enrichment_enabled":       true,
		"schedule_cron":            "15 * * * *",
		"schedule_timezone":        "Asia/Shanghai",
		"join_key_type":            string(DirectoryJoinKeyUsername),
		"scheduled_department_ids": []interface{}{"2", "3"},
		"scheduled_include_nested": true,
	})
	if err != nil {
		t.Fatalf("BuildScheduledDirectoryEnrichmentPlan() error = %v", err)
	}
	if !plan.Enabled {
		t.Fatal("plan.Enabled = false, want true")
	}
	if plan.ProviderRequest["include_nested"] != true {
		t.Fatalf("include_nested = %#v, want true", plan.ProviderRequest["include_nested"])
	}
	rawDepartmentIDs, ok := plan.ProviderRequest["department_ids"].([]string)
	if !ok {
		t.Fatalf("department_ids type = %T, want []string", plan.ProviderRequest["department_ids"])
	}
	if len(rawDepartmentIDs) != 2 || rawDepartmentIDs[0] != "2" || rawDepartmentIDs[1] != "3" {
		t.Fatalf("department_ids = %#v, want [2 3]", rawDepartmentIDs)
	}
}
