package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/ent"
)

func TestAuthProviderGeneration_StableAndDetectsRuntimeChanges(t *testing.T) {
	updatedAt := time.Date(2026, time.July, 18, 12, 0, 0, 123000, time.UTC)
	baseline := &ent.AuthProvider{
		ID:        "provider-1",
		AuthType:  "oidc",
		Config:    map[string]interface{}{"issuer": "https://issuer.example.com", "client_id": "shepherd"},
		UpdatedAt: updatedAt,
	}
	generation, err := CaptureAuthProviderGeneration(baseline)
	if err != nil {
		t.Fatalf("capture generation: %v", err)
	}
	if err := generation.Validate(&ent.AuthProvider{
		ID:        baseline.ID,
		AuthType:  baseline.AuthType,
		Config:    map[string]interface{}{"client_id": "shepherd", "issuer": "https://issuer.example.com"},
		UpdatedAt: updatedAt.In(time.FixedZone("same-instant", 8*60*60)),
	}); err != nil {
		t.Fatalf("equivalent provider generation was rejected: %v", err)
	}

	tests := []struct {
		name string
		row  *ent.AuthProvider
	}{
		{
			name: "auth type",
			row: &ent.AuthProvider{
				ID: baseline.ID, AuthType: "ldap", Config: baseline.Config, UpdatedAt: updatedAt,
			},
		},
		{
			name: "config",
			row: &ent.AuthProvider{
				ID: baseline.ID, AuthType: baseline.AuthType, Config: map[string]interface{}{"issuer": "https://new.example.com"}, UpdatedAt: updatedAt,
			},
		},
		{
			name: "updated at",
			row: &ent.AuthProvider{
				ID: baseline.ID, AuthType: baseline.AuthType, Config: baseline.Config, UpdatedAt: updatedAt.Add(time.Microsecond),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := generation.Validate(tt.row); !errors.Is(err, ErrAuthProviderGenerationChanged) {
				t.Fatalf("Validate() error = %v, want generation changed", err)
			}
		})
	}
}

func TestAuthProviderGeneration_NormalizesPostgreSQLTimestampPrecision(t *testing.T) {
	preInsertTime := time.Date(2026, time.July, 18, 12, 0, 0, 123456789, time.UTC)
	baseline := &ent.AuthProvider{
		ID:        "provider-precision",
		AuthType:  "oidc",
		Config:    map[string]interface{}{"issuer": "https://issuer.example.com"},
		UpdatedAt: preInsertTime,
	}
	generation, err := CaptureAuthProviderGeneration(baseline)
	if err != nil {
		t.Fatalf("capture generation: %v", err)
	}

	reloaded := *baseline
	reloaded.UpdatedAt = preInsertTime.Truncate(time.Microsecond)
	if err := generation.Validate(&reloaded); err != nil {
		t.Fatalf("PostgreSQL microsecond round trip changed generation: %v", err)
	}

	reloaded.UpdatedAt = reloaded.UpdatedAt.Add(time.Microsecond)
	if err := generation.Validate(&reloaded); !errors.Is(err, ErrAuthProviderGenerationChanged) {
		t.Fatalf("Validate() error = %v, want generation changed after one microsecond", err)
	}
}

func TestAuthProviderGeneration_StateBindingIsKeyedStableAndGenerationSpecific(t *testing.T) {
	updatedAt := time.Date(2026, time.July, 18, 12, 0, 0, 123456000, time.UTC)
	baseline := &ent.AuthProvider{
		ID:        "provider-state-binding",
		AuthType:  "oidc",
		Config:    map[string]interface{}{"issuer": "https://issuer.example.com", "client_secret": "sentinel-secret"},
		UpdatedAt: updatedAt,
	}
	generation, err := CaptureAuthProviderGeneration(baseline)
	if err != nil {
		t.Fatalf("capture generation: %v", err)
	}

	key := []byte("state-signing-key-0123456789abcdef")
	binding, err := generation.StateBinding(key)
	if err != nil {
		t.Fatalf("bind generation: %v", err)
	}
	if binding == "" || strings.Contains(binding, "sentinel-secret") || strings.Contains(binding, baseline.ID) {
		t.Fatalf("binding is not opaque: %q", binding)
	}
	if bindingErr := generation.ValidateStateBinding(key, binding); bindingErr != nil {
		t.Fatalf("validate matching binding: %v", bindingErr)
	}

	equivalent, err := CaptureAuthProviderGeneration(&ent.AuthProvider{
		ID:        baseline.ID,
		AuthType:  baseline.AuthType,
		Config:    map[string]interface{}{"client_secret": "sentinel-secret", "issuer": "https://issuer.example.com"},
		UpdatedAt: updatedAt.In(time.FixedZone("same-instant", 8*60*60)),
	})
	if err != nil {
		t.Fatalf("capture equivalent generation: %v", err)
	}
	equivalentBinding, err := equivalent.StateBinding(key)
	if err != nil {
		t.Fatalf("bind equivalent generation: %v", err)
	}
	if equivalentBinding != binding {
		t.Fatalf("equivalent generation binding = %q, want %q", equivalentBinding, binding)
	}

	changed, err := CaptureAuthProviderGeneration(&ent.AuthProvider{
		ID:        baseline.ID,
		AuthType:  baseline.AuthType,
		Config:    map[string]interface{}{"issuer": "https://changed.example.com", "client_secret": "sentinel-secret"},
		UpdatedAt: updatedAt,
	})
	if err != nil {
		t.Fatalf("capture changed generation: %v", err)
	}
	if err := changed.ValidateStateBinding(key, binding); !errors.Is(err, ErrAuthProviderGenerationChanged) {
		t.Fatalf("changed generation binding error = %v, want generation changed", err)
	}
	if err := generation.ValidateStateBinding([]byte("different-state-signing-key"), binding); !errors.Is(err, ErrAuthProviderGenerationChanged) {
		t.Fatalf("different key binding error = %v, want generation changed", err)
	}
	if _, err := generation.StateBinding(nil); err == nil {
		t.Fatal("StateBinding() accepted an empty key")
	}
}
