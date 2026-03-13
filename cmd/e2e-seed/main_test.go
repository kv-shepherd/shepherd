package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
)

const sampleSeedKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: cluster-a
  cluster:
    server: https://cluster-a.example.invalid:6443
contexts:
- name: ctx-a
  context:
    cluster: cluster-a
    user: user-a
current-context: ctx-a
users:
- name: user-a
  user:
    token: sample
`

func TestResolveClusterSeedInput_UsesBase64Kubeconfig(t *testing.T) {
	t.Setenv("E2E_KUBECONFIG_B64", base64.StdEncoding.EncodeToString([]byte(sampleSeedKubeconfig)))
	t.Setenv("E2E_KUBECONFIG_PATH", "")

	input, err := resolveClusterSeedInput(fixtureConfig{ClusterAPIURL: defaultClusterAPIURL})
	if err != nil {
		t.Fatalf("resolveClusterSeedInput: %v", err)
	}
	if got, want := string(input.Kubeconfig), sampleSeedKubeconfig; got != want {
		t.Fatalf("kubeconfig = %q, want %q", got, want)
	}
	if got, want := input.APIServer, "https://cluster-a.example.invalid:6443"; got != want {
		t.Fatalf("api server = %q, want %q", got, want)
	}
	if got, want := input.Status, entcluster.StatusHEALTHY; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
}

func TestResolveClusterSeedInput_UsesPathWhenBase64Missing(t *testing.T) {
	dir := t.TempDir()
	kubeconfigPath := filepath.Join(dir, "dev-kubeconfig.yaml")
	if err := os.WriteFile(kubeconfigPath, []byte(sampleSeedKubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	t.Setenv("E2E_KUBECONFIG_B64", "")
	t.Setenv("E2E_KUBECONFIG_PATH", kubeconfigPath)

	input, err := resolveClusterSeedInput(fixtureConfig{ClusterAPIURL: defaultClusterAPIURL})
	if err != nil {
		t.Fatalf("resolveClusterSeedInput: %v", err)
	}
	if got, want := string(input.Kubeconfig), sampleSeedKubeconfig; got != want {
		t.Fatalf("kubeconfig = %q, want %q", got, want)
	}
	if got, want := input.APIServer, "https://cluster-a.example.invalid:6443"; got != want {
		t.Fatalf("api server = %q, want %q", got, want)
	}
}

func TestResolveClusterSeedInput_FallsBackToStubWithoutKubeconfig(t *testing.T) {
	t.Setenv("E2E_KUBECONFIG_B64", "")
	t.Setenv("E2E_KUBECONFIG_PATH", "")

	input, err := resolveClusterSeedInput(fixtureConfig{ClusterAPIURL: defaultClusterAPIURL})
	if err != nil {
		t.Fatalf("resolveClusterSeedInput: %v", err)
	}
	if got, want := input.Status, entcluster.StatusUNREACHABLE; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := input.APIServer, defaultClusterAPIURL; got != want {
		t.Fatalf("api server = %q, want %q", got, want)
	}
}

func TestLiveInstanceSizeFixtures_UseNestedSpecOverrides(t *testing.T) {
	fixtures := liveInstanceSizeFixtures(fixtureConfig{SizeName: defaultSizeName})

	for _, fixture := range fixtures {
		if len(fixture.SpecOverrides) == 0 {
			continue
		}

		if len(fixture.SpecOverrides) != 1 {
			t.Fatalf("%s: spec_overrides should have exactly one top-level key, got %d", fixture.Name, len(fixture.SpecOverrides))
		}

		rawSpec, ok := fixture.SpecOverrides["spec"]
		if !ok {
			t.Fatalf("%s: spec_overrides must use nested root key \"spec\"", fixture.Name)
		}

		specMap, ok := rawSpec.(map[string]interface{})
		if !ok {
			t.Fatalf("%s: spec_overrides[\"spec\"] must be an object, got %T", fixture.Name, rawSpec)
		}

		if len(specMap) == 0 {
			t.Fatalf("%s: spec_overrides[\"spec\"] must not be empty", fixture.Name)
		}

		for key := range fixture.SpecOverrides {
			if key != "spec" {
				t.Fatalf("%s: flat top-level key %q is not allowed in spec_overrides", fixture.Name, key)
			}
		}
	}
}

func TestLiveInstanceSizeFixtures_DefaultSmallProfile(t *testing.T) {
	fixtures := liveInstanceSizeFixtures(fixtureConfig{SizeName: defaultSizeName})

	var small *instanceSizeFixture
	for i := range fixtures {
		if fixtures[i].Name == defaultSizeName {
			small = &fixtures[i]
			break
		}
	}
	if small == nil {
		t.Fatalf("fixture %q not found", defaultSizeName)
	}

	if small.CPUCores != 1 || small.CPURequest != 0.5 {
		t.Fatalf("small cpu profile = %.1f/%.1f, want 1.0/0.5", small.CPUCores, small.CPURequest)
	}
	if small.MemoryGi != 2 || small.MemoryRequestGi != 1 {
		t.Fatalf("small memory profile = %.1f/%.1f, want 2.0/1.0", small.MemoryGi, small.MemoryRequestGi)
	}
	if small.DiskGB != 60 {
		t.Fatalf("small disk = %d, want 60", small.DiskGB)
	}
}
