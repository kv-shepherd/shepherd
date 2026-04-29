package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
)

type fakeLDAPConnection struct {
	bindCalls []struct {
		username string
		password string
	}
	bindErrs      []error
	searchResults []*ldap.SearchResult
	searchErrs    []error
}

func (c *fakeLDAPConnection) Bind(username, password string) error {
	c.bindCalls = append(c.bindCalls, struct {
		username string
		password string
	}{username: username, password: password})
	if len(c.bindErrs) > 0 {
		err := c.bindErrs[0]
		c.bindErrs = c.bindErrs[1:]
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *fakeLDAPConnection) Search(*ldap.SearchRequest) (*ldap.SearchResult, error) {
	if len(c.searchErrs) > 0 {
		err := c.searchErrs[0]
		c.searchErrs = c.searchErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(c.searchResults) == 0 {
		return &ldap.SearchResult{}, nil
	}
	result := c.searchResults[0]
	c.searchResults = c.searchResults[1:]
	return result, nil
}

func (c *fakeLDAPConnection) StartTLS(*tls.Config) error { return nil }

func (c *fakeLDAPConnection) Close() error { return nil }

func TestLDAPAuthProviderSupportsDirectorySyncCapability(t *testing.T) {
	adapter := ResolveAuthProviderAdminAdapter("ldap")
	if adapter == nil {
		t.Fatal("ldap auth provider adapter is nil")
	}
	if _, ok := adapter.(DirectorySyncCapability); !ok {
		t.Fatal("ldap auth provider adapter does not implement DirectorySyncCapability")
	}
	if _, ok := adapter.(ScheduledDirectoryEnrichmentCapability); !ok {
		t.Fatal("ldap auth provider adapter does not implement ScheduledDirectoryEnrichmentCapability")
	}
	if _, ok := adapter.(AuthCredentialCapability); !ok {
		t.Fatal("ldap auth provider adapter does not implement AuthCredentialCapability")
	}
}

func TestLDAPAuthProviderTestConnectionBindsConfiguredDN(t *testing.T) {
	conn := &fakeLDAPConnection{}
	adapter := &ldapAuthProviderAdapter{
		dial: func(serverURL string, tlsEnabled, tlsSkipVerify bool, timeout time.Duration) (ldapConnection, error) {
			if serverURL != "ldaps://ldap.example.com:636" {
				t.Fatalf("serverURL = %q, want %q", serverURL, "ldaps://ldap.example.com:636")
			}
			if !tlsEnabled {
				t.Fatal("tlsEnabled = false, want true")
			}
			if tlsSkipVerify {
				t.Fatal("tlsSkipVerify = true, want false")
			}
			if timeout != defaultLDAPRequestTimeout {
				t.Fatalf("timeout = %s, want %s", timeout, defaultLDAPRequestTimeout)
			}
			return conn, nil
		},
	}

	ok, message, err := adapter.TestConnection(t.Context(), map[string]interface{}{
		"server_url":       "ldaps://ldap.example.com:636",
		"bind_dn":          "cn=admin,dc=example,dc=com",
		"bind_password":    "secret",
		"base_dn":          "ou=users,dc=example,dc=com",
		"directory_filter": "(objectClass=person)",
	})
	if err != nil {
		t.Fatalf("test connection error: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, message = %q", message)
	}
	if len(conn.bindCalls) != 1 {
		t.Fatalf("bindCalls = %d, want 1", len(conn.bindCalls))
	}
	if got := conn.bindCalls[0].username; got != "cn=admin,dc=example,dc=com" {
		t.Fatalf("bind username = %q", got)
	}
}

func TestLDAPAuthProviderListDirectoryUsersMapsEntries(t *testing.T) {
	conn := &fakeLDAPConnection{
		searchResults: []*ldap.SearchResult{
			{
				Entries: []*ldap.Entry{
					ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{
						"uid":             {"alice"},
						"displayName":     {"Alice Example"},
						"mail":            {"alice@example.com"},
						"memberOf":        {"cn=dev,ou=groups,dc=example,dc=com", "cn=ops,ou=groups,dc=example,dc=com"},
						"telephoneNumber": {"12345"},
					}),
				},
			},
		},
	}
	adapter := &ldapAuthProviderAdapter{
		dial: func(string, bool, bool, time.Duration) (ldapConnection, error) {
			return conn, nil
		},
	}

	records, err := adapter.ListDirectoryUsers(t.Context(), map[string]interface{}{
		"server_url":             "ldaps://ldap.example.com:636",
		"bind_dn":                "cn=admin,dc=example,dc=com",
		"bind_password":          "secret",
		"base_dn":                "ou=users,dc=example,dc=com",
		"username_attribute":     "uid",
		"display_name_attribute": "displayName",
		"email_attribute":        "mail",
	}, map[string]interface{}{
		"limit": float64(1),
	})
	if err != nil {
		t.Fatalf("list directory users: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	record := records[0]
	if record.ExternalID != "uid=alice,ou=users,dc=example,dc=com" {
		t.Fatalf("ExternalID = %q", record.ExternalID)
	}
	if record.Username != "alice" {
		t.Fatalf("Username = %q", record.Username)
	}
	if record.DisplayName != "Alice Example" {
		t.Fatalf("DisplayName = %q", record.DisplayName)
	}
	if record.Email != "alice@example.com" {
		t.Fatalf("Email = %q", record.Email)
	}
	if len(record.Cohorts) != 2 {
		t.Fatalf("Cohorts = %#v", record.Cohorts)
	}
	if record.Cohorts[0].Kind != "group" || record.Cohorts[0].Key != "cn=dev,ou=groups,dc=example,dc=com" {
		t.Fatalf("first cohort = %#v", record.Cohorts[0])
	}
	if got := record.Attributes["ldap_dn"]; got != "uid=alice,ou=users,dc=example,dc=com" {
		t.Fatalf("ldap_dn = %#v", got)
	}
	if got := record.Attributes["telephoneNumber"]; got != "12345" {
		t.Fatalf("telephoneNumber = %#v", got)
	}
}

func TestLDAPAuthProviderBuildsScheduledDirectoryEnrichmentPlan(t *testing.T) {
	adapter := &ldapAuthProviderAdapter{}

	plan, err := adapter.BuildScheduledDirectoryEnrichmentPlan(context.Background(), map[string]interface{}{
		"enrichment_enabled": true,
		"base_dn":            "ou=users,dc=example,dc=com",
		"directory_filter":   "(objectClass=inetOrgPerson)",
		"schedule_cron":      "15 * * * *",
		"schedule_timezone":  "Asia/Shanghai",
		"join_key_type":      string(DirectoryJoinKeyUsername),
	})
	if err != nil {
		t.Fatalf("build scheduled enrichment plan: %v", err)
	}
	if !plan.Enabled {
		t.Fatal("plan.Enabled = false, want true")
	}
	if plan.ProviderRequest["search_base"] != "ou=users,dc=example,dc=com" {
		t.Fatalf("search_base = %#v", plan.ProviderRequest["search_base"])
	}
	if plan.ProviderRequest["directory_filter"] != "(objectClass=inetOrgPerson)" {
		t.Fatalf("directory_filter = %#v", plan.ProviderRequest["directory_filter"])
	}
}

func TestLDAPAuthProviderValidateConfig_RejectsLdapSchemeInReleaseMode(t *testing.T) {
	t.Setenv("GIN_MODE", "release")

	adapter := &ldapAuthProviderAdapter{}
	err := adapter.ValidateConfig(map[string]interface{}{
		"server_url":       "ldap://ldap.example.com:389",
		"tls_enabled":      true,
		"bind_dn":          "cn=admin,dc=example,dc=com",
		"bind_password":    "secret",
		"base_dn":          "ou=users,dc=example,dc=com",
		"directory_filter": "(objectClass=person)",
	})
	if err == nil {
		t.Fatal("ValidateConfig() expected ldaps-only error in release mode, got nil")
	}
	if !strings.Contains(err.Error(), "ldaps://") {
		t.Fatalf("ValidateConfig() error = %v, want ldaps guidance", err)
	}
}

func TestLDAPAuthProviderAuthenticateCredentials_SearchesThenBindsUser(t *testing.T) {
	conn := &fakeLDAPConnection{
		searchResults: []*ldap.SearchResult{
			{
				Entries: []*ldap.Entry{
					ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{
						"uid":         {"alice"},
						"displayName": {"Alice Example"},
						"mail":        {"alice@example.com"},
						"memberOf":    {"cn=dev,ou=groups,dc=example,dc=com"},
					}),
				},
			},
		},
	}
	adapter := &ldapAuthProviderAdapter{
		dial: func(string, bool, bool, time.Duration) (ldapConnection, error) {
			return conn, nil
		},
	}

	result, err := adapter.AuthenticateCredentials(t.Context(), map[string]interface{}{
		"server_url":    "ldaps://ldap.example.com:636",
		"bind_dn":       "cn=admin,dc=example,dc=com",
		"bind_password": "secret",
		"base_dn":       "ou=users,dc=example,dc=com",
		"user_filter":   "(uid=%s)",
	}, AuthCredentialRequest{
		Credentials: map[string]interface{}{
			"username": "alice",
			"password": "user-secret",
		},
	})
	if err != nil {
		t.Fatalf("AuthenticateCredentials() error = %v", err)
	}
	if result.Username != "alice" || result.DisplayName != "Alice Example" {
		t.Fatalf("result = %#v", result)
	}
	if len(conn.bindCalls) != 2 {
		t.Fatalf("bindCalls = %d, want 2", len(conn.bindCalls))
	}
	if got := conn.bindCalls[1].username; got != "uid=alice,ou=users,dc=example,dc=com" {
		t.Fatalf("user bind DN = %q", got)
	}
}

func TestLDAPAuthProviderValidateConfig_RejectsPlainLDAPWithoutTLS(t *testing.T) {
	adapter := &ldapAuthProviderAdapter{}

	err := adapter.ValidateConfig(map[string]interface{}{
		"server_url":    "ldap://ldap.example.com:389",
		"bind_dn":       "cn=admin,dc=example,dc=com",
		"base_dn":       "ou=users,dc=example,dc=com",
		"tls_enabled":   false,
		"bind_password": "secret",
	})
	if err == nil {
		t.Fatal("ValidateConfig() error = nil, want rejection for plain LDAP without TLS")
	}
}

func TestLDAPAuthProviderAuthenticateCredentials_InvalidPassword(t *testing.T) {
	conn := &fakeLDAPConnection{
		bindErrs: []error{nil, errors.New("invalid credentials")},
		searchResults: []*ldap.SearchResult{
			{
				Entries: []*ldap.Entry{
					ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{
						"uid":         {"alice"},
						"displayName": {"Alice Example"},
					}),
				},
			},
		},
	}
	adapter := &ldapAuthProviderAdapter{
		dial: func(string, bool, bool, time.Duration) (ldapConnection, error) {
			return conn, nil
		},
	}

	_, err := adapter.AuthenticateCredentials(t.Context(), map[string]interface{}{
		"server_url":    "ldaps://ldap.example.com:636",
		"bind_dn":       "cn=admin,dc=example,dc=com",
		"bind_password": "secret",
		"base_dn":       "ou=users,dc=example,dc=com",
		"user_filter":   "(uid=%s)",
	}, AuthCredentialRequest{
		Credentials: map[string]interface{}{
			"username": "alice",
			"password": "wrong",
		},
	})
	var credentialErr *AuthCredentialError
	if !errors.As(err, &credentialErr) {
		t.Fatalf("err = %v, want AuthCredentialError", err)
	}
	if credentialErr.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("code = %q, want INVALID_CREDENTIALS", credentialErr.Code)
	}
}

func TestLDAPDirectoryRequestRejectsInvalidFilter(t *testing.T) {
	_, _, _, err := ldapDirectoryRequestFromConfig(map[string]interface{}{
		"base_dn": "ou=users,dc=example,dc=com",
	}, map[string]interface{}{
		"directory_filter": "invalid(",
	})
	var requestErr *DirectorySyncRequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("err = %v, want DirectorySyncRequestError", err)
	}
}
