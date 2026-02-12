package app

import (
	"testing"

	"kv-shepherd.io/shepherd/internal/config"
)

func TestBuildCORSConfig_DefaultsToAllowlistWhenOriginsEmpty(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			AllowedOrigins:        nil,
			AllowCredentials:      true,
			UnsafeAllowAllOrigins: false,
		},
	}

	got := buildCORSConfig(cfg)
	if got.AllowAllOrigins {
		t.Fatalf("AllowAllOrigins = %v, want false", got.AllowAllOrigins)
	}
	if !got.AllowCredentials {
		t.Fatalf("AllowCredentials = %v, want true", got.AllowCredentials)
	}
	if len(got.AllowOrigins) != 2 {
		t.Fatalf("len(AllowOrigins) = %d, want 2", len(got.AllowOrigins))
	}
}

func TestBuildCORSConfig_StripsWildcardUnlessUnsafeFlagEnabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			AllowedOrigins:        []string{"*", "https://example.com"},
			AllowCredentials:      true,
			UnsafeAllowAllOrigins: false,
		},
	}

	got := buildCORSConfig(cfg)
	if got.AllowAllOrigins {
		t.Fatalf("AllowAllOrigins = %v, want false", got.AllowAllOrigins)
	}
	if len(got.AllowOrigins) != 1 || got.AllowOrigins[0] != "https://example.com" {
		t.Fatalf("AllowOrigins = %#v, want []string{\"https://example.com\"}", got.AllowOrigins)
	}
}

func TestBuildCORSConfig_UnsafeAllowAllDisablesCredentials(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			AllowedOrigins:        []string{"*"},
			AllowCredentials:      true,
			UnsafeAllowAllOrigins: true,
		},
	}

	got := buildCORSConfig(cfg)
	if !got.AllowAllOrigins {
		t.Fatalf("AllowAllOrigins = %v, want true", got.AllowAllOrigins)
	}
	if got.AllowCredentials {
		t.Fatalf("AllowCredentials = %v, want false", got.AllowCredentials)
	}
	if len(got.AllowOrigins) != 0 {
		t.Fatalf("AllowOrigins = %#v, want empty", got.AllowOrigins)
	}
}
