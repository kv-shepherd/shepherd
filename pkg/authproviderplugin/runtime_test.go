package authproviderplugin_test

import (
	"context"
	"errors"
	"testing"

	"kv-shepherd.io/shepherd/pkg/authproviderplugin"
)

type runtimeOnlyProvider struct{}

func (p *runtimeOnlyProvider) StartLogin(_ context.Context, _ map[string]interface{}, _ authproviderplugin.AuthStartRequest) (*authproviderplugin.AuthStartResponse, error) {
	return &authproviderplugin.AuthStartResponse{RedirectURL: "https://example.invalid/login"}, nil
}

func (p *runtimeOnlyProvider) CompleteLogin(_ context.Context, _ map[string]interface{}, _ authproviderplugin.AuthCallbackRequest) (*authproviderplugin.AuthResult, error) {
	return &authproviderplugin.AuthResult{
		ExternalID:  "user-1",
		Username:    "alice",
		DisplayName: "Alice",
		Enabled:     true,
	}, nil
}

type credentialRuntimeProvider struct{}

func (p *credentialRuntimeProvider) AuthenticateCredentials(_ context.Context, _ map[string]interface{}, req authproviderplugin.AuthCredentialRequest) (*authproviderplugin.AuthResult, error) {
	if req.Credentials["username"] == "" {
		return nil, authproviderplugin.NewAuthCredentialError("INVALID_CREDENTIALS", "username is required")
	}
	return &authproviderplugin.AuthResult{
		ExternalID:  "ldap-user-1",
		Username:    "alice",
		DisplayName: "Alice",
		Enabled:     true,
	}, nil
}

type directoryOnlyProvider struct{}

func (p *directoryOnlyProvider) DescribeDirectorySync() authproviderplugin.DirectorySyncDescriptor {
	return authproviderplugin.DirectorySyncDescriptor{
		DisplayName:     "Example Directory",
		SupportsPreview: true,
	}
}

func (p *directoryOnlyProvider) PreviewDirectorySync(_ context.Context, _, _ map[string]interface{}) (*authproviderplugin.DirectorySyncPreview, error) {
	return &authproviderplugin.DirectorySyncPreview{
		Items: []authproviderplugin.DirectoryPreviewItem{
			{
				Record: authproviderplugin.DirectoryUserRecord{
					ExternalID:  "ext-1",
					Username:    "alice",
					DisplayName: "Alice",
				},
				Conflicts: []authproviderplugin.DirectoryConflict{
					{
						Code: authproviderplugin.DirectoryConflictUsernameConflict,
					},
				},
			},
		},
	}, nil
}

func (p *directoryOnlyProvider) ListDirectoryUsers(_ context.Context, _, req map[string]interface{}) ([]authproviderplugin.DirectoryUserRecord, error) {
	if _, ok := req["selector"]; !ok {
		return nil, authproviderplugin.NewDirectorySyncRequestError("selector is required")
	}
	return []authproviderplugin.DirectoryUserRecord{
		{
			ExternalID:  "ext-1",
			Username:    "alice",
			DisplayName: "Alice",
		},
	}, nil
}

func (p *directoryOnlyProvider) BuildScheduledDirectoryEnrichmentPlan(_ context.Context, _ map[string]interface{}) (*authproviderplugin.ScheduledDirectoryEnrichmentPlan, error) {
	return &authproviderplugin.ScheduledDirectoryEnrichmentPlan{
		Enabled:      true,
		Mode:         authproviderplugin.DirectoryEnrichmentModeEnrichExistingOnly,
		JoinKeyType:  authproviderplugin.DirectoryJoinKeyUsername,
		ScheduleCron: "0 * * * *",
	}, nil
}

func TestNewAuthStartError_AsPublicType(t *testing.T) {
	t.Parallel()

	err := authproviderplugin.NewAuthStartError("AUTH_LOGIN_MODE_UNAVAILABLE", "unsupported here")
	var startErr *authproviderplugin.AuthStartError
	if !errors.As(err, &startErr) {
		t.Fatalf("errors.As() = false, want true")
	}
	if startErr.Code != "AUTH_LOGIN_MODE_UNAVAILABLE" {
		t.Fatalf("code = %q, want AUTH_LOGIN_MODE_UNAVAILABLE", startErr.Code)
	}
}

func TestNewDirectorySyncRequestError_AsPublicType(t *testing.T) {
	t.Parallel()

	err := authproviderplugin.NewDirectorySyncRequestError("selector is required")
	var requestErr *authproviderplugin.DirectorySyncRequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("errors.As() = false, want true")
	}
	if requestErr.Message != "selector is required" {
		t.Fatalf("message = %q, want selector is required", requestErr.Message)
	}
}

func TestNewAuthCredentialError_AsPublicType(t *testing.T) {
	t.Parallel()

	err := authproviderplugin.NewAuthCredentialError("INVALID_CREDENTIALS", "invalid credentials")
	var credentialErr *authproviderplugin.AuthCredentialError
	if !errors.As(err, &credentialErr) {
		t.Fatalf("errors.As() = false, want true")
	}
	if credentialErr.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("code = %q, want INVALID_CREDENTIALS", credentialErr.Code)
	}
}

func TestDirectoryConflictConstants_ArePublic(t *testing.T) {
	t.Parallel()

	if got := string(authproviderplugin.DirectoryConflictSameExternalIdentity); got != "same_external_identity" {
		t.Fatalf("DirectoryConflictSameExternalIdentity = %q, want same_external_identity", got)
	}
	if got := string(authproviderplugin.DirectoryConflictUsernameConflict); got != "username_conflict" {
		t.Fatalf("DirectoryConflictUsernameConflict = %q, want username_conflict", got)
	}
	if got := string(authproviderplugin.DirectoryConflictEmailConflict); got != "email_conflict" {
		t.Fatalf("DirectoryConflictEmailConflict = %q, want email_conflict", got)
	}
	if got := string(authproviderplugin.DirectoryConflictAmbiguousExisting); got != "ambiguous_existing_user" {
		t.Fatalf("DirectoryConflictAmbiguousExisting = %q, want ambiguous_existing_user", got)
	}
}

func TestPublicSDKCapabilityInterfaces_AreImplementableWithoutInternalImports(t *testing.T) {
	t.Parallel()

	var runtimeCap authproviderplugin.RuntimeCapability = &runtimeOnlyProvider{}
	var credentialCap authproviderplugin.CredentialRuntimeCapability = &credentialRuntimeProvider{}
	var directoryCap authproviderplugin.DirectorySyncCapability = &directoryOnlyProvider{}
	var scheduledCap authproviderplugin.ScheduledDirectoryEnrichmentCapability = &directoryOnlyProvider{}

	startResp, err := runtimeCap.StartLogin(context.Background(), nil, authproviderplugin.AuthStartRequest{})
	if err != nil {
		t.Fatalf("StartLogin() error = %v", err)
	}
	if startResp.RedirectURL == "" {
		t.Fatal("expected redirect URL")
	}

	authResult, err := credentialCap.AuthenticateCredentials(context.Background(), nil, authproviderplugin.AuthCredentialRequest{
		Credentials: map[string]interface{}{"username": "alice"},
	})
	if err != nil {
		t.Fatalf("AuthenticateCredentials() error = %v", err)
	}
	if authResult.Username != "alice" {
		t.Fatalf("authResult.Username = %q, want alice", authResult.Username)
	}

	preview, err := directoryCap.PreviewDirectorySync(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("PreviewDirectorySync() error = %v", err)
	}
	if len(preview.Items) != 1 {
		t.Fatalf("len(preview.Items) = %d, want 1", len(preview.Items))
	}

	_, err = directoryCap.ListDirectoryUsers(context.Background(), nil, nil)
	var requestErr *authproviderplugin.DirectorySyncRequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("ListDirectoryUsers() error type = %T, want *DirectorySyncRequestError", err)
	}

	plan, err := scheduledCap.BuildScheduledDirectoryEnrichmentPlan(context.Background(), nil)
	if err != nil {
		t.Fatalf("BuildScheduledDirectoryEnrichmentPlan() error = %v", err)
	}
	if plan.Mode != authproviderplugin.DirectoryEnrichmentModeEnrichExistingOnly {
		t.Fatalf("plan.Mode = %q, want %q", plan.Mode, authproviderplugin.DirectoryEnrichmentModeEnrichExistingOnly)
	}
}
