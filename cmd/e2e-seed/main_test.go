package main

import (
	"os"
	"path/filepath"
	"testing"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
)

func TestResolveClusterSeedInput_WithoutKubeconfigFallsBackToStub(t *testing.T) {
	t.Setenv("E2E_KUBECONFIG_B64", "")
	t.Setenv("E2E_KUBECONFIG_PATH", "")

	input, err := resolveClusterSeedInput(fixtureConfig{ClusterAPIURL: defaultClusterAPIURL})
	if err != nil {
		t.Fatalf("resolveClusterSeedInput() error = %v", err)
	}
	if input.Status != entcluster.StatusUNREACHABLE {
		t.Fatalf("status = %s, want %s", input.Status, entcluster.StatusUNREACHABLE)
	}
	if shouldSeedLiveVMFixtures(input) {
		t.Fatal("shouldSeedLiveVMFixtures() = true, want false for stub input")
	}
}

func TestResolveClusterSeedInput_WithKubeconfigSeedsLiveFixtures(t *testing.T) {
	dir := t.TempDir()
	kubeconfigPath := filepath.Join(dir, "kubeconfig.yaml")
	const kubeconfig = `apiVersion: v1
kind: Config
clusters:
  - name: dev
    cluster:
      server: https://cluster.example.test:6443
contexts:
  - name: dev
    context:
      cluster: dev
      user: dev
current-context: dev
users:
  - name: dev
    user:
      token: token
`
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("E2E_KUBECONFIG_B64", "")
	t.Setenv("E2E_KUBECONFIG_PATH", kubeconfigPath)

	input, err := resolveClusterSeedInput(fixtureConfig{})
	if err != nil {
		t.Fatalf("resolveClusterSeedInput() error = %v", err)
	}
	if input.Status != entcluster.StatusHEALTHY {
		t.Fatalf("status = %s, want %s", input.Status, entcluster.StatusHEALTHY)
	}
	if input.APIServer != "https://cluster.example.test:6443" {
		t.Fatalf("APIServer = %q, want https://cluster.example.test:6443", input.APIServer)
	}
	if !shouldSeedLiveVMFixtures(input) {
		t.Fatal("shouldSeedLiveVMFixtures() = false, want true for live kubeconfig")
	}
}
