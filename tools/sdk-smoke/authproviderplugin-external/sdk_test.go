package authproviderplugintest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"kv-shepherd.io/shepherd/pkg/authproviderplugin"
)

type smokeAdapter struct{}

func (smokeAdapter) Type() string {
	return "external-sdk-smoke"
}

func (smokeAdapter) ValidateConfig(config map[string]interface{}) error {
	if config == nil {
		return authproviderplugin.NewDirectorySyncRequestError("config must not be nil")
	}
	return nil
}

func (smokeAdapter) TestConnection(_ context.Context, _ map[string]interface{}) (bool, string, error) {
	return true, "ok", nil
}

func (smokeAdapter) SampleFields(_ context.Context, _ map[string]interface{}) ([]authproviderplugin.AdminSampleField, error) {
	return []authproviderplugin.AdminSampleField{
		{
			Field:       "department",
			ValueType:   "string",
			UniqueCount: 1,
			Sample:      []string{"engineering"},
		},
	}, nil
}

func (smokeAdapter) Describe() authproviderplugin.AdminTypeDescriptor {
	return authproviderplugin.AdminTypeDescriptor{
		Type:        "external-sdk-smoke",
		DisplayName: "External SDK Smoke",
		BuiltIn:     false,
	}
}

func (smokeAdapter) StartLogin(_ context.Context, _ map[string]interface{}, _ authproviderplugin.AuthStartRequest) (*authproviderplugin.AuthStartResponse, error) {
	return &authproviderplugin.AuthStartResponse{RedirectURL: "https://example.invalid/login"}, nil
}

func (smokeAdapter) CompleteLogin(_ context.Context, _ map[string]interface{}, _ authproviderplugin.AuthCallbackRequest) (*authproviderplugin.AuthResult, error) {
	return &authproviderplugin.AuthResult{
		ExternalID:  "ext-1",
		Username:    "alice",
		DisplayName: "Alice",
		Enabled:     true,
	}, nil
}

func (smokeAdapter) DescribeDirectorySync() authproviderplugin.DirectorySyncDescriptor {
	return authproviderplugin.DirectorySyncDescriptor{
		DisplayName:     "External Directory",
		SupportsPreview: true,
	}
}

func (smokeAdapter) PreviewDirectorySync(_ context.Context, _, _ map[string]interface{}) (*authproviderplugin.DirectorySyncPreview, error) {
	return &authproviderplugin.DirectorySyncPreview{
		Items: []authproviderplugin.DirectoryPreviewItem{
			{
				Record: authproviderplugin.DirectoryUserRecord{
					ExternalID:  "ext-1",
					Username:    "alice",
					DisplayName: "Alice",
				},
			},
		},
	}, nil
}

func (smokeAdapter) ListDirectoryUsers(_ context.Context, _, _ map[string]interface{}) ([]authproviderplugin.DirectoryUserRecord, error) {
	return []authproviderplugin.DirectoryUserRecord{
		{
			ExternalID:  "ext-1",
			Username:    "alice",
			DisplayName: "Alice",
		},
	}, nil
}

func (smokeAdapter) BuildScheduledDirectoryEnrichmentPlan(_ context.Context, _ map[string]interface{}) (*authproviderplugin.ScheduledDirectoryEnrichmentPlan, error) {
	return &authproviderplugin.ScheduledDirectoryEnrichmentPlan{
		Enabled:      true,
		Mode:         authproviderplugin.DirectoryEnrichmentModeEnrichExistingOnly,
		JoinKeyType:  authproviderplugin.DirectoryJoinKeyUsername,
		ScheduleCron: "0 * * * *",
	}, nil
}

func TestExternalModuleCanConsumePublicAuthProviderSDK(t *testing.T) {
	t.Parallel()

	var adminAdapter authproviderplugin.AdminAdapter = smokeAdapter{}
	var describer authproviderplugin.AdminAdapterDescriber = smokeAdapter{}
	var runtimeCap authproviderplugin.RuntimeCapability = smokeAdapter{}
	var directoryCap authproviderplugin.DirectorySyncCapability = smokeAdapter{}
	var enrichmentCap authproviderplugin.ScheduledDirectoryEnrichmentCapability = smokeAdapter{}

	if err := authproviderplugin.RegisterAdminAdapter(adminAdapter); err != nil {
		t.Fatalf("RegisterAdminAdapter() error = %v", err)
	}

	types := authproviderplugin.ListRegisteredAdminTypes()
	if !slices.ContainsFunc(types, func(item authproviderplugin.AdminTypeDescriptor) bool {
		return item.Type == "external-sdk-smoke"
	}) {
		t.Fatalf("ListRegisteredAdminTypes() missing external-sdk-smoke: %#v", types)
	}

	if got := describer.Describe().DisplayName; got != "External SDK Smoke" {
		t.Fatalf("Describe().DisplayName = %q, want External SDK Smoke", got)
	}

	startResp, err := runtimeCap.StartLogin(context.Background(), nil, authproviderplugin.AuthStartRequest{})
	if err != nil {
		t.Fatalf("StartLogin() error = %v", err)
	}
	if startResp.RedirectURL == "" {
		t.Fatal("expected redirect URL")
	}

	preview, err := directoryCap.PreviewDirectorySync(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("PreviewDirectorySync() error = %v", err)
	}
	if len(preview.Items) != 1 {
		t.Fatalf("len(preview.Items) = %d, want 1", len(preview.Items))
	}

	plan, err := enrichmentCap.BuildScheduledDirectoryEnrichmentPlan(context.Background(), nil)
	if err != nil {
		t.Fatalf("BuildScheduledDirectoryEnrichmentPlan() error = %v", err)
	}
	if plan.Mode != authproviderplugin.DirectoryEnrichmentModeEnrichExistingOnly {
		t.Fatalf("plan.Mode = %q, want %q", plan.Mode, authproviderplugin.DirectoryEnrichmentModeEnrichExistingOnly)
	}
}

func TestExternalModuleSeesStructuredPublicErrors(t *testing.T) {
	t.Parallel()

	err := authproviderplugin.NewAuthStartError("AUTH_LOGIN_MODE_UNAVAILABLE", "unsupported")
	var startErr *authproviderplugin.AuthStartError
	if !errors.As(err, &startErr) {
		t.Fatalf("errors.As() = false, want true")
	}
	if startErr.Code != "AUTH_LOGIN_MODE_UNAVAILABLE" {
		t.Fatalf("startErr.Code = %q, want AUTH_LOGIN_MODE_UNAVAILABLE", startErr.Code)
	}
}
