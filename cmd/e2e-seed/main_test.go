package main

import "testing"

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("E2E_TEST_KEY", "")
	if got := envOrDefault("E2E_TEST_KEY", "fallback"); got != "fallback" {
		t.Fatalf("envOrDefault empty = %q, want fallback", got)
	}

	t.Setenv("E2E_TEST_KEY", "  configured  ")
	if got := envOrDefault("E2E_TEST_KEY", "fallback"); got != "configured" {
		t.Fatalf("envOrDefault value = %q, want configured", got)
	}
}

func TestLoadFixtureConfig_Defaults(t *testing.T) {
	t.Setenv("E2E_ADMIN_USERNAME", "")
	t.Setenv("E2E_ADMIN_PASSWORD", "")
	t.Setenv("E2E_NAMESPACE", "")

	cfg := loadFixtureConfig()
	if cfg.AdminUsername != defaultAdminUsername {
		t.Fatalf("AdminUsername = %q, want %q", cfg.AdminUsername, defaultAdminUsername)
	}
	if cfg.AdminPassword != defaultAdminPassword {
		t.Fatalf("AdminPassword = %q, want %q", cfg.AdminPassword, defaultAdminPassword)
	}
	if cfg.NamespaceName != defaultNamespaceName {
		t.Fatalf("NamespaceName = %q, want %q", cfg.NamespaceName, defaultNamespaceName)
	}
}

func TestLoadFixtureConfig_Overrides(t *testing.T) {
	t.Setenv("E2E_ADMIN_USERNAME", "tester")
	t.Setenv("E2E_ADMIN_PASSWORD", "password-1")
	t.Setenv("E2E_NAMESPACE", "ns-live")
	t.Setenv("E2E_VM_RUNNING_ID", "vm-live-x")

	cfg := loadFixtureConfig()
	if cfg.AdminUsername != "tester" {
		t.Fatalf("AdminUsername = %q, want tester", cfg.AdminUsername)
	}
	if cfg.AdminPassword != "password-1" {
		t.Fatalf("AdminPassword = %q, want password-1", cfg.AdminPassword)
	}
	if cfg.NamespaceName != "ns-live" {
		t.Fatalf("NamespaceName = %q, want ns-live", cfg.NamespaceName)
	}
	if cfg.RunningVMID != "vm-live-x" {
		t.Fatalf("RunningVMID = %q, want vm-live-x", cfg.RunningVMID)
	}
}
