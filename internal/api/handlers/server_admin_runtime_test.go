package handlers

import (
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestGetAuthProviderRuntimeDescriptor_ReturnsCredentialModeForLDAP(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)

	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-runtime-ldap").
		SetName("LDAP Runtime").
		SetAuthType("ldap").
		SetConfig(map[string]interface{}{}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	reqCtx, reqW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+providerRow.ID+"/runtime",
		"",
		"admin-1",
		[]string{"auth_provider:read"},
	)
	srv.GetAuthProviderRuntimeDescriptor(reqCtx, providerRow.ID)
	if reqW.Code != http.StatusOK {
		t.Fatalf("runtime descriptor status = %d, want %d, body=%s", reqW.Code, http.StatusOK, reqW.Body.String())
	}

	var resp generated.AuthProviderRuntimeDescriptor
	mustDecodeJSON(t, reqW.Body.Bytes(), &resp)
	if !resp.Supported {
		t.Fatalf("supported = false, want true")
	}
	if resp.SupportsRedirect {
		t.Fatalf("supports_redirect = true, want false")
	}
	if !resp.SupportsCredentials {
		t.Fatalf("supports_credentials = false, want true")
	}
	if resp.RequiresPublicBaseUrl {
		t.Fatalf("requires_public_base_url = true, want false")
	}
	if len(resp.LoginModes) != 1 {
		t.Fatalf("login mode count = %d, want 1", len(resp.LoginModes))
	}
	if resp.LoginModes[0].Interaction != generated.Credentials {
		t.Fatalf("interaction = %q, want %q", resp.LoginModes[0].Interaction, generated.Credentials)
	}
}

func TestGetAuthProviderRuntimeDescriptor_ReturnsRedirectModeForOIDC(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)

	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-runtime-oidc").
		SetName("OIDC Runtime").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	reqCtx, reqW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+providerRow.ID+"/runtime",
		"",
		"admin-1",
		[]string{"auth_provider:read"},
	)
	srv.GetAuthProviderRuntimeDescriptor(reqCtx, providerRow.ID)
	if reqW.Code != http.StatusOK {
		t.Fatalf("runtime descriptor status = %d, want %d, body=%s", reqW.Code, http.StatusOK, reqW.Body.String())
	}

	var resp generated.AuthProviderRuntimeDescriptor
	mustDecodeJSON(t, reqW.Body.Bytes(), &resp)
	if !resp.Supported {
		t.Fatalf("supported = false, want true")
	}
	if !resp.SupportsRedirect {
		t.Fatalf("supports_redirect = false, want true")
	}
	if resp.SupportsCredentials {
		t.Fatalf("supports_credentials = true, want false")
	}
	if !resp.RequiresPublicBaseUrl {
		t.Fatalf("requires_public_base_url = false, want true")
	}
	if len(resp.LoginModes) != 1 {
		t.Fatalf("login mode count = %d, want 1", len(resp.LoginModes))
	}
}
