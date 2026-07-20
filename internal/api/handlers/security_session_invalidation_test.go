package handlers

import (
	"net/http"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/internal/service"
)

func assertSessionVersionIncremented(
	t *testing.T,
	authSessions *service.AuthSessionManager,
	userID string,
	before int64,
) {
	t.Helper()
	after, err := authSessions.CurrentSessionVersion(t.Context(), userID)
	if err != nil {
		t.Fatalf("CurrentSessionVersion(%q) after update: %v", userID, err)
	}
	if after != before+1 {
		t.Fatalf("CurrentSessionVersion(%q) = %d, want %d", userID, after, before+1)
	}
}

func TestSecuritySensitiveNoOpUpdatesStillInvalidateSessions(t *testing.T) {
	t.Run("user enabled field", func(t *testing.T) {
		srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "security_noop_user")
		userRow, err := client.User.Create().
			SetID("security-noop-user").
			SetUsername("security.noop.user").
			SetEnabled(true).
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed user: %v", err)
		}
		before, err := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
		if err != nil {
			t.Fatalf("seed session version: %v", err)
		}

		requestContext, response := newAuthedGinContext(
			t,
			http.MethodPatch,
			"/admin/users/"+userRow.ID,
			`{"enabled":true}`,
			"security-admin",
			[]string{"user:manage"},
		)
		srv.UpdateUser(requestContext, userRow.ID)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		assertSessionVersionIncremented(t, authSessions, userRow.ID, before)
	})

	t.Run("role enabled field", func(t *testing.T) {
		srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "security_noop_role")
		userRow, err := client.User.Create().
			SetID("security-noop-role-user").
			SetUsername("security.noop.role.user").
			SetEnabled(true).
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed user: %v", err)
		}
		roleRow, err := client.Role.Create().
			SetID("security-noop-role").
			SetName("Security No-op Role").
			SetPermissions([]string{"vm:read"}).
			SetEnabled(true).
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed role: %v", err)
		}
		if _, bindingErr := client.RoleBinding.Create().
			SetID("security-noop-role-binding").
			SetUser(userRow).
			SetRole(roleRow).
			SetScopeType(scopeTypeGlobal).
			SetCreatedBy("seed").
			Save(t.Context()); bindingErr != nil {
			t.Fatalf("seed role binding: %v", bindingErr)
		}
		before, err := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
		if err != nil {
			t.Fatalf("seed session version: %v", err)
		}

		requestContext, response := newAuthedGinContext(
			t,
			http.MethodPatch,
			"/admin/roles/"+roleRow.ID,
			`{"enabled":true}`,
			"security-admin",
			[]string{"rbac:manage"},
		)
		srv.UpdateRole(requestContext, roleRow.ID)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		assertSessionVersionIncremented(t, authSessions, userRow.ID, before)
	})

	t.Run("auth provider enabled field", func(t *testing.T) {
		srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "security_noop_provider")
		providerRow, err := client.AuthProvider.Create().
			SetID("security-noop-provider").
			SetName("Security No-op Provider").
			SetAuthType("oidc").
			SetConfig(map[string]interface{}{}).
			SetEnabled(true).
			SetCreatedBy("seed").
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed auth provider: %v", err)
		}
		userRow, err := client.User.Create().
			SetID("security-noop-provider-user").
			SetUsername("security.noop.provider.user").
			SetEnabled(true).
			SetAuthProviderID(providerRow.ID).
			SetExternalID("security-noop-provider-external").
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed linked user: %v", err)
		}
		before, err := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
		if err != nil {
			t.Fatalf("seed session version: %v", err)
		}

		requestContext, response := newAuthedGinContext(
			t,
			http.MethodPatch,
			"/admin/auth-providers/"+providerRow.ID,
			`{"enabled":true}`,
			"security-admin",
			[]string{"auth_provider:update"},
		)
		srv.UpdateAuthProvider(requestContext, providerRow.ID)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		assertSessionVersionIncremented(t, authSessions, userRow.ID, before)
	})
}

func TestAuthProviderConfigUpdatesInvalidateLinkedUserSessions(t *testing.T) {
	tests := []struct {
		name     string
		clientID string
	}{
		{name: "config changed", clientID: "client-rotated"},
		{name: "config no-op", clientID: "client-original"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
				t,
				"security_provider_config_"+test.clientID,
			)
			storedConfig, err := srv.authProviderConfig.EncryptForStorage("oidc", map[string]interface{}{
				"issuer_url":    "https://issuer.example.com",
				"client_id":     "client-original",
				"client_secret": "secret-original",
			})
			if err != nil {
				t.Fatalf("encrypt provider config: %v", err)
			}
			providerRow, err := client.AuthProvider.Create().
				SetID("security-provider-config-" + test.clientID).
				SetName("Security Provider Config " + test.clientID).
				SetAuthType("oidc").
				SetConfig(storedConfig).
				SetEnabled(true).
				SetCreatedBy("seed").
				Save(t.Context())
			if err != nil {
				t.Fatalf("seed auth provider: %v", err)
			}
			userRow, err := client.User.Create().
				SetID("security-provider-config-user-" + test.clientID).
				SetUsername("security.provider.config." + test.clientID).
				SetEnabled(true).
				SetAuthProviderID(providerRow.ID).
				SetExternalID("security-provider-config-external-" + test.clientID).
				Save(t.Context())
			if err != nil {
				t.Fatalf("seed linked user: %v", err)
			}
			before, err := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
			if err != nil {
				t.Fatalf("seed session version: %v", err)
			}

			requestContext, response := newAuthedGinContext(
				t,
				http.MethodPatch,
				"/admin/auth-providers/"+providerRow.ID,
				`{"config":{"client_id":"`+test.clientID+`"}}`,
				"security-admin",
				[]string{"auth_provider:update"},
			)
			srv.UpdateAuthProvider(requestContext, providerRow.ID)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusOK, response.Body.String())
			}
			assertSessionVersionIncremented(t, authSessions, userRow.ID, before)
		})
	}
}

func TestAuthProviderConfigUpdateRollsBackWhenSessionInvalidationFails(t *testing.T) {
	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"security_provider_config_rollback",
	)
	storedConfig, err := srv.authProviderConfig.EncryptForStorage("oidc", map[string]interface{}{
		"issuer_url":    "https://issuer.example.com",
		"client_id":     "client-original",
		"client_secret": "secret-original",
	})
	if err != nil {
		t.Fatalf("encrypt provider config: %v", err)
	}
	providerRow, err := client.AuthProvider.Create().
		SetID("security-provider-config-rollback").
		SetName("Security Provider Config Rollback").
		SetAuthType("oidc").
		SetConfig(storedConfig).
		SetEnabled(true).
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed auth provider: %v", err)
	}
	userRow, err := client.User.Create().
		SetID("security-provider-config-rollback-user").
		SetUsername("security.provider.config.rollback").
		SetEnabled(true).
		SetAuthProviderID(providerRow.ID).
		SetExternalID("security-provider-config-rollback-external").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed linked user: %v", err)
	}
	beforeVersions := installAuthSessionVersionBumpFailure(t, srv, authSessions, userRow.ID)

	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+providerRow.ID,
		`{"config":{"client_id":"client-rotated"}}`,
		"security-admin",
		[]string{"auth_provider:update"},
	)
	srv.UpdateAuthProvider(requestContext, providerRow.ID)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	assertAuthSessionVersionBumpFailureTriggered(t, srv)

	reloaded, err := client.AuthProvider.Get(t.Context(), providerRow.ID)
	if err != nil {
		t.Fatalf("reload provider after rollback: %v", err)
	}
	plainConfig, err := srv.authProviderConfig.DecryptForUse(reloaded.AuthType, reloaded.Config)
	if err != nil {
		t.Fatalf("decrypt provider config after rollback: %v", err)
	}
	if got := plainConfig["client_id"]; got != "client-original" {
		t.Fatalf("provider client_id after rollback = %#v, want client-original", got)
	}
	if !reloaded.UpdatedAt.Equal(providerRow.UpdatedAt.Truncate(time.Microsecond)) {
		t.Fatalf("provider updated_at changed after rollback: got=%s want=%s", reloaded.UpdatedAt, providerRow.UpdatedAt)
	}
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)
}
