package main

import (
	"encoding/base64"
	"strings"
	"testing"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
)

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
	t.Setenv("E2E_CLUSTER_API_SERVER", "")

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
	if cfg.ClusterAPIURL != defaultClusterAPIURL {
		t.Fatalf("ClusterAPIURL = %q, want %q", cfg.ClusterAPIURL, defaultClusterAPIURL)
	}
}

func TestLoadFixtureConfig_Overrides(t *testing.T) {
	t.Setenv("E2E_ADMIN_USERNAME", "tester")
	t.Setenv("E2E_ADMIN_PASSWORD", "password-1")
	t.Setenv("E2E_NAMESPACE", "ns-live")
	t.Setenv("E2E_CLUSTER_API_SERVER", "https://override.cluster.invalid")
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
	if cfg.ClusterAPIURL != "https://override.cluster.invalid" {
		t.Fatalf("ClusterAPIURL = %q, want override URL", cfg.ClusterAPIURL)
	}
}

func TestResolveClusterSeedInput_DefaultFallback(t *testing.T) {
	t.Setenv("E2E_KUBECONFIG_B64", "")

	input, err := resolveClusterSeedInput(fixtureConfig{ClusterAPIURL: "https://seed-api.invalid"})
	if err != nil {
		t.Fatalf("resolveClusterSeedInput() unexpected error: %v", err)
	}
	if input.APIServer != "https://seed-api.invalid" {
		t.Fatalf("APIServer = %q, want %q", input.APIServer, "https://seed-api.invalid")
	}
	if input.Status != entcluster.StatusUNREACHABLE {
		t.Fatalf("Status = %q, want %q", input.Status, entcluster.StatusUNREACHABLE)
	}
	if got := strings.TrimSpace(string(input.Kubeconfig)); got == "" {
		t.Fatal("Kubeconfig should use non-empty fallback stub")
	}
}

func TestResolveClusterSeedInput_InvalidBase64(t *testing.T) {
	t.Setenv("E2E_KUBECONFIG_B64", "%%%not-base64%%%")

	_, err := resolveClusterSeedInput(fixtureConfig{ClusterAPIURL: defaultClusterAPIURL})
	if err == nil {
		t.Fatal("resolveClusterSeedInput() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode E2E_KUBECONFIG_B64") {
		t.Fatalf("error = %v, want decode failure", err)
	}
}

func TestResolveClusterSeedInput_UsesKubeconfigServerWhenDefaultAPIURL(t *testing.T) {
	raw := strings.TrimSpace(`
apiVersion: v1
kind: Config
clusters:
  - name: c1
    cluster:
      server: https://kube-from-config.invalid
contexts:
  - name: c1
    context:
      cluster: c1
      user: u1
current-context: c1
users:
  - name: u1
    user:
      token: test-token
`)
	t.Setenv("E2E_KUBECONFIG_B64", base64.StdEncoding.EncodeToString([]byte(raw)))

	input, err := resolveClusterSeedInput(fixtureConfig{ClusterAPIURL: defaultClusterAPIURL})
	if err != nil {
		t.Fatalf("resolveClusterSeedInput() unexpected error: %v", err)
	}
	if input.APIServer != "https://kube-from-config.invalid" {
		t.Fatalf("APIServer = %q, want kubeconfig server", input.APIServer)
	}
	if input.Status != entcluster.StatusHEALTHY {
		t.Fatalf("Status = %q, want %q", input.Status, entcluster.StatusHEALTHY)
	}
	if len(input.Kubeconfig) == 0 {
		t.Fatal("Kubeconfig should not be empty")
	}
}
