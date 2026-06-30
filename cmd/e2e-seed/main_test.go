package main

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"kv-shepherd.io/shepherd/ent"
	entauthprovider "kv-shepherd.io/shepherd/ent/authprovider"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entclusterpolicy "kv-shepherd.io/shepherd/ent/clusterpolicy"
	entdomainevent "kv-shepherd.io/shepherd/ent/domainevent"
	entinstancesize "kv-shepherd.io/shepherd/ent/instancesize"
	entnotification "kv-shepherd.io/shepherd/ent/notification"
	entresourcerolebinding "kv-shepherd.io/shepherd/ent/resourcerolebinding"
	entrole "kv-shepherd.io/shepherd/ent/role"
	entrolebinding "kv-shepherd.io/shepherd/ent/rolebinding"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entuser "kv-shepherd.io/shepherd/ent/user"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.MustStartDockerPG(m)
}

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

func TestLoadFixtureConfig_SecondUserOverrides(t *testing.T) {
	t.Setenv("E2E_ADMIN_USERNAME", "admin")
	t.Setenv("E2E_ADMIN_PASSWORD", "admin")
	t.Setenv("E2E_ADMIN_EMAIL", "admin@example.test")
	t.Setenv("E2E_SECOND_USERNAME", "test")
	t.Setenv("E2E_SECOND_PASSWORD", "test")
	t.Setenv("E2E_SECOND_EMAIL", "test@example.test")
	t.Setenv("E2E_SECOND_DISPLAY_NAME", "Test User")
	t.Setenv("E2E_SECOND_ROLE_NAME", "TestEngineer")

	cfg := loadFixtureConfig()
	if cfg.AdminUsername != "admin" || cfg.AdminPassword != "admin" || cfg.AdminEmail != "admin@example.test" {
		t.Fatalf("admin fixture config = %#v, want explicit admin overrides", cfg)
	}
	if cfg.SecondUsername != "test" {
		t.Fatalf("SecondUsername = %q, want test", cfg.SecondUsername)
	}
	if cfg.SecondPassword != "test" {
		t.Fatalf("SecondPassword = %q, want test", cfg.SecondPassword)
	}
	if cfg.SecondEmail != "test@example.test" {
		t.Fatalf("SecondEmail = %q, want test@example.test", cfg.SecondEmail)
	}
	if cfg.SecondDisplayName != "Test User" {
		t.Fatalf("SecondDisplayName = %q, want Test User", cfg.SecondDisplayName)
	}
	if cfg.SecondRoleName != "TestEngineer" {
		t.Fatalf("SecondRoleName = %q, want TestEngineer", cfg.SecondRoleName)
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "one", value: "1", want: true},
		{name: "true mixed case", value: " TrUe ", want: true},
		{name: "yes", value: "yes", want: true},
		{name: "on", value: "on", want: true},
		{name: "false", value: "false", want: false},
		{name: "empty", value: "", want: false},
		{name: "unknown", value: "enabled", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("E2E_BOOL_TEST", tc.value)
			if got := envBool("E2E_BOOL_TEST"); got != tc.want {
				t.Fatalf("envBool(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestEffectiveE2ESeedTimeout(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      time.Duration
		wantError string
	}{
		{name: "default", want: defaultE2ESeedTimeout},
		{name: "explicit duration", value: "5m30s", want: 5*time.Minute + 30*time.Second},
		{name: "trimmed duration", value: " 90s ", want: 90 * time.Second},
		{name: "invalid duration", value: "soon", wantError: "parse E2E_SEED_TIMEOUT"},
		{name: "zero duration", value: "0s", wantError: "must be greater than 0"},
		{name: "negative duration", value: "-1s", wantError: "must be greater than 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("E2E_SEED_TIMEOUT", tc.value)

			got, err := effectiveE2ESeedTimeout()
			if tc.wantError != "" {
				if err == nil {
					t.Fatal("effectiveE2ESeedTimeout() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("effectiveE2ESeedTimeout() error = %v, want substring %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("effectiveE2ESeedTimeout() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("effectiveE2ESeedTimeout() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestLoadSeedKubeconfigBytesFromBase64(t *testing.T) {
	const kubeconfig = "apiVersion: v1\nkind: Config\n"
	t.Setenv("E2E_KUBECONFIG_B64", base64.StdEncoding.EncodeToString([]byte(kubeconfig)))
	t.Setenv("E2E_KUBECONFIG_PATH", "")

	got, err := loadSeedKubeconfigBytes()
	if err != nil {
		t.Fatalf("loadSeedKubeconfigBytes() error = %v", err)
	}
	if string(got) != kubeconfig {
		t.Fatalf("loadSeedKubeconfigBytes() = %q, want %q", string(got), kubeconfig)
	}
}

func TestLoadSeedKubeconfigBytesErrors(t *testing.T) {
	tests := []struct {
		name    string
		rawB64  string
		path    string
		content string
		want    string
		wantErr error
	}{
		{
			name:   "invalid base64",
			rawB64: "%%%",
			want:   "decode E2E_KUBECONFIG_B64",
		},
		{
			name:   "empty decoded base64",
			rawB64: base64.StdEncoding.EncodeToString([]byte("   ")),
			want:   "E2E_KUBECONFIG_B64 decoded to empty content",
		},
		{
			name:    "empty path file",
			path:    "kubeconfig.yaml",
			content: "   ",
			want:    "is empty",
		},
		{
			name:    "no configured kubeconfig",
			wantErr: errNoSeedKubeconfig,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("E2E_KUBECONFIG_B64", tc.rawB64)
			if tc.path != "" {
				dir := t.TempDir()
				path := filepath.Join(dir, tc.path)
				if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
				t.Setenv("E2E_KUBECONFIG_PATH", path)
			} else {
				t.Setenv("E2E_KUBECONFIG_PATH", "")
			}

			_, err := loadSeedKubeconfigBytes()
			if err == nil {
				t.Fatal("loadSeedKubeconfigBytes() error = nil, want error")
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("loadSeedKubeconfigBytes() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadSeedKubeconfigBytes() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestFirstKubeconfigServer(t *testing.T) {
	cfg := &clientcmdapi.Config{
		CurrentContext: "current",
		Contexts: map[string]*clientcmdapi.Context{
			"current": {Cluster: "current-cluster"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"current-cluster": {Server: " https://current.example.test "},
			"alpha":           {Server: "https://alpha.example.test"},
		},
	}
	if got := firstKubeconfigServer(cfg); got != "https://current.example.test" {
		t.Fatalf("firstKubeconfigServer(current) = %q, want current cluster server", got)
	}

	cfg.CurrentContext = "missing"
	cfg.Clusters["alpha"].Server = " https://alpha.example.test "
	cfg.Clusters["beta"] = &clientcmdapi.Cluster{Server: "https://beta.example.test"}
	if got := firstKubeconfigServer(cfg); got != "https://alpha.example.test" {
		t.Fatalf("firstKubeconfigServer(fallback) = %q, want sorted first non-empty server", got)
	}

	if got := firstKubeconfigServer(nil); got != "" {
		t.Fatalf("firstKubeconfigServer(nil) = %q, want empty", got)
	}
}

func TestResolveClusterSeedInputUsesExplicitAPIURL(t *testing.T) {
	t.Setenv("E2E_KUBECONFIG_B64", base64.StdEncoding.EncodeToString([]byte(`apiVersion: v1
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
`)))
	t.Setenv("E2E_KUBECONFIG_PATH", "")

	input, err := resolveClusterSeedInput(fixtureConfig{ClusterAPIURL: "https://override.example.test:6443"})
	if err != nil {
		t.Fatalf("resolveClusterSeedInput() error = %v", err)
	}
	if input.APIServer != "https://override.example.test:6443" {
		t.Fatalf("APIServer = %q, want explicit override", input.APIServer)
	}
	if input.Status != entcluster.StatusHEALTHY {
		t.Fatalf("Status = %s, want %s", input.Status, entcluster.StatusHEALTHY)
	}
}

func TestLiveTemplateFixture(t *testing.T) {
	got := liveTemplateFixture(fixtureConfig{
		NamespaceName: "team-a",
		TemplateName:  "ubuntu-e2e",
	})

	if got.Name != "ubuntu-e2e" {
		t.Fatalf("Name = %q, want ubuntu-e2e", got.Name)
	}
	if got.CatalogScope != enttemplate.CatalogScopeAll {
		t.Fatalf("CatalogScope = %s, want %s", got.CatalogScope, enttemplate.CatalogScopeAll)
	}
	if !strings.Contains(got.Description, "team-a") {
		t.Fatalf("Description = %q, want namespace context", got.Description)
	}
	if got.ImageURL != defaultTemplateImageURL || got.CloudInit != defaultTemplateCloudInit {
		t.Fatalf("template image/cloud-init did not use defaults: %#v", got)
	}

	withoutNamespace := liveTemplateFixture(fixtureConfig{TemplateName: "ubuntu-e2e"})
	if withoutNamespace.Description != "Live E2E Ubuntu template" {
		t.Fatalf("empty namespace description = %q, want generic description", withoutNamespace.Description)
	}
}

func TestLiveInstanceSizeFixtures(t *testing.T) {
	got := liveInstanceSizeFixtures(fixtureConfig{SizeName: "custom-small"})
	if len(got) != 6 {
		t.Fatalf("liveInstanceSizeFixtures length = %d, want 6", len(got))
	}

	byName := make(map[string]instanceSizeFixture, len(got))
	for _, fixture := range got {
		byName[fixture.Name] = fixture
		if fixture.CatalogScope != entinstancesize.CatalogScopeAll {
			t.Fatalf("%s CatalogScope = %s, want %s", fixture.Name, fixture.CatalogScope, entinstancesize.CatalogScopeAll)
		}
	}
	if byName["custom-small"].CPUCores != 1 || byName["custom-small"].MemoryGi != 2 {
		t.Fatalf("custom-small fixture = %#v, want small baseline", byName["custom-small"])
	}
	if !byName["e2e-dedicated"].DedicatedCPU {
		t.Fatal("e2e-dedicated DedicatedCPU = false, want true")
	}
	if !byName["e2e-gpu"].RequiresGPU {
		t.Fatal("e2e-gpu RequiresGPU = false, want true")
	}
	if !byName["e2e-hugepages"].RequiresHugepages || byName["e2e-hugepages"].HugepagesSize != "2Mi" {
		t.Fatalf("e2e-hugepages fixture = %#v, want 2Mi hugepages requirement", byName["e2e-hugepages"])
	}
	if !byName["e2e-sriov"].RequiresSriov {
		t.Fatal("e2e-sriov RequiresSriov = false, want true")
	}

	wantDedicatedOverride := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"domain": map[string]interface{}{
						"cpu": map[string]interface{}{
							"dedicatedCpuPlacement": true,
						},
					},
				},
			},
		},
	}
	if !reflect.DeepEqual(byName["e2e-dedicated"].SpecOverrides, wantDedicatedOverride) {
		t.Fatalf("dedicated SpecOverrides = %#v, want %#v", byName["e2e-dedicated"].SpecOverrides, wantDedicatedOverride)
	}
}

func TestNestedSpecOverrides(t *testing.T) {
	spec := map[string]interface{}{"template": map[string]interface{}{"spec": "value"}}
	got := nestedSpecOverrides(spec)
	want := map[string]interface{}{"spec": spec}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nestedSpecOverrides() = %#v, want %#v", got, want)
	}
}

func TestE2ESeedCatalogAndVMFixturesAreIdempotent(t *testing.T) {
	t.Setenv("E2E_KUBECONFIG_B64", "")
	t.Setenv("E2E_KUBECONFIG_PATH", "")

	ctx := context.Background()
	client := testutil.OpenEntPostgres(t, "e2e_seed_catalog")
	fx := testFixtureConfig()

	if err := ensureNamespaceRegistry(ctx, client, fx); err != nil {
		t.Fatalf("ensureNamespaceRegistry() error = %v", err)
	}
	clusterID, clusterErr := ensureCluster(ctx, client, fx)
	if clusterErr != nil {
		t.Fatalf("ensureCluster() error = %v", clusterErr)
	}
	if err := ensureClusterPolicy(ctx, client, clusterID, fx); err != nil {
		t.Fatalf("ensureClusterPolicy() error = %v", err)
	}
	systemID, systemErr := ensureSystem(ctx, client, fx)
	if systemErr != nil {
		t.Fatalf("ensureSystem() error = %v", systemErr)
	}
	serviceID, serviceErr := ensureService(ctx, client, fx, systemID)
	if serviceErr != nil {
		t.Fatalf("ensureService() error = %v", serviceErr)
	}
	if err := ensureTemplate(ctx, client, fx); err != nil {
		t.Fatalf("ensureTemplate() error = %v", err)
	}
	if err := ensureInstanceSize(ctx, client, fx); err != nil {
		t.Fatalf("ensureInstanceSize() error = %v", err)
	}
	if err := ensureExistingTemplate(ctx, client, fx.TemplateName); err != nil {
		t.Fatalf("ensureExistingTemplate() error = %v", err)
	}
	if err := ensureExistingInstanceSize(ctx, client, fx.SizeName); err != nil {
		t.Fatalf("ensureExistingInstanceSize() error = %v", err)
	}
	if err := ensureExistingTemplate(ctx, client, "missing-template"); err == nil || !strings.Contains(err.Error(), "template \"missing-template\" not found") {
		t.Fatalf("ensureExistingTemplate(missing) error = %v, want missing template diagnostic", err)
	}
	if err := ensureExistingInstanceSize(ctx, client, "missing-size"); err == nil || !strings.Contains(err.Error(), "instance size \"missing-size\" not found") {
		t.Fatalf("ensureExistingInstanceSize(missing) error = %v, want missing instance size diagnostic", err)
	}

	count, countErr := client.InstanceSize.Query().
		Where(entinstancesize.CreatedByEQ(seedActor)).
		Count(ctx)
	if countErr != nil {
		t.Fatalf("count seeded instance sizes: %v", countErr)
	}
	if count != len(liveInstanceSizeFixtures(fx)) {
		t.Fatalf("seeded instance sizes = %d, want %d", count, len(liveInstanceSizeFixtures(fx)))
	}

	if _, err := client.InstanceSize.Create().
		SetID("e2e-obsolete-size").
		SetName("e2e-obsolete").
		SetDisplayName("Obsolete").
		SetCPUCores(1).
		SetCPURequest(1).
		SetMemoryGi(1).
		SetMemoryRequestGi(1).
		SetDiskGB(10).
		SetCreatedBy(seedActor).
		Save(ctx); err != nil {
		t.Fatalf("create obsolete instance size: %v", err)
	}
	if err := resetManagedInstanceSizes(ctx, client); err != nil {
		t.Fatalf("resetManagedInstanceSizes() error = %v", err)
	}
	count, countErr = client.InstanceSize.Query().
		Where(entinstancesize.CreatedByEQ(seedActor)).
		Count(ctx)
	if countErr != nil {
		t.Fatalf("count reset instance sizes: %v", countErr)
	}
	if count != 0 {
		t.Fatalf("seeded instance sizes after reset = %d, want 0", count)
	}
	if err := ensureInstanceSize(ctx, client, fx); err != nil {
		t.Fatalf("ensureInstanceSize() after reset error = %v", err)
	}

	if err := ensureVM(ctx, client, fx.RunningVMID, "vm-live", "01", entvm.StatusRUNNING, fx.NamespaceName, clusterID, serviceID, fx.AdminUsername); err != nil {
		t.Fatalf("ensureVM(running) error = %v", err)
	}
	if err := ensureVM(ctx, client, fx.StoppedVMID, "vm-stopped", "02", entvm.StatusSTOPPED, fx.NamespaceName, clusterID, serviceID, fx.AdminUsername); err != nil {
		t.Fatalf("ensureVM(stopped) error = %v", err)
	}
	vmCount, vmCountErr := client.VM.Query().
		Where(entvm.IDIn(fx.RunningVMID, fx.StoppedVMID)).
		Count(ctx)
	if vmCountErr != nil {
		t.Fatalf("count seeded VMs: %v", vmCountErr)
	}
	if vmCount != 2 {
		t.Fatalf("seeded VM count = %d, want 2", vmCount)
	}
	if err := deleteSeedVMFixtures(ctx, client, fx); err != nil {
		t.Fatalf("deleteSeedVMFixtures() error = %v", err)
	}
	vmCount, vmCountErr = client.VM.Query().
		Where(entvm.IDIn(fx.RunningVMID, fx.StoppedVMID)).
		Count(ctx)
	if vmCountErr != nil {
		t.Fatalf("count deleted seed VMs: %v", vmCountErr)
	}
	if vmCount != 0 {
		t.Fatalf("seeded VM count after cleanup = %d, want 0", vmCount)
	}

	if _, err := client.Cluster.UpdateOneID(clusterID).
		SetDisplayName("Stale Cluster").
		SetStatus(entcluster.StatusHEALTHY).
		SetEnabled(false).
		Save(ctx); err != nil {
		t.Fatalf("stale cluster update: %v", err)
	}
	if _, err := client.ClusterPolicy.Update().
		Where(entclusterpolicy.ClusterIDEQ(clusterID)).
		SetAllowGpu(false).
		SetAllowHugepages(false).
		SetAllowedHugepagesSizes([]string{"1Gi"}).
		SetAllowedCloneSourceNamespaces([]string{"legacy"}).
		Save(ctx); err != nil {
		t.Fatalf("stale cluster policy update: %v", err)
	}
	if _, err := client.Template.Update().
		Where(enttemplate.NameEQ(fx.TemplateName)).
		SetDisplayName("Stale Template").
		SetSourceType("cdi_pvc_clone").
		ClearImageURL().
		SetPvcName("legacy-pvc").
		SetPvcNamespace("legacy-ns").
		SetEnabled(false).
		Save(ctx); err != nil {
		t.Fatalf("stale template update: %v", err)
	}
	if _, err := client.InstanceSize.Update().
		Where(entinstancesize.NameEQ(fx.SizeName)).
		SetCPUCores(8).
		SetCPURequest(7).
		SetMemoryGi(9).
		SetMemoryRequestGi(9).
		SetSpecOverrides(map[string]interface{}{"stale": true}).
		SetEnabled(false).
		Save(ctx); err != nil {
		t.Fatalf("stale instance size update: %v", err)
	}

	rerunClusterID, rerunClusterErr := ensureCluster(ctx, client, fx)
	if rerunClusterErr != nil {
		t.Fatalf("ensureCluster() rerun error = %v", rerunClusterErr)
	}
	if rerunClusterID != clusterID {
		t.Fatalf("ensureCluster() rerun ID = %q, want %q", rerunClusterID, clusterID)
	}
	if err := ensureClusterPolicy(ctx, client, clusterID, fx); err != nil {
		t.Fatalf("ensureClusterPolicy() rerun error = %v", err)
	}
	if err := ensureTemplate(ctx, client, fx); err != nil {
		t.Fatalf("ensureTemplate() rerun error = %v", err)
	}
	if err := ensureInstanceSize(ctx, client, fx); err != nil {
		t.Fatalf("ensureInstanceSize() rerun error = %v", err)
	}

	cluster := mustGetCluster(ctx, t, client, clusterID)
	if cluster.Status != entcluster.StatusUNREACHABLE || !cluster.Enabled || cluster.APIServerURL != defaultClusterAPIURL {
		t.Fatalf("cluster after rerun = status %s enabled %v api %q, want unreachable/enabled/default api", cluster.Status, cluster.Enabled, cluster.APIServerURL)
	}
	policy, policyErr := client.ClusterPolicy.Query().
		Where(entclusterpolicy.ClusterIDEQ(clusterID)).
		Only(ctx)
	if policyErr != nil {
		t.Fatalf("query cluster policy: %v", policyErr)
	}
	if !policy.AllowGpu || !policy.AllowHugepages {
		t.Fatalf("cluster policy allow flags = gpu %v hugepages %v, want both true", policy.AllowGpu, policy.AllowHugepages)
	}
	if want := []string{defaultCloneNSName, fx.NamespaceName}; !reflect.DeepEqual(policy.AllowedCloneSourceNamespaces, want) {
		t.Fatalf("AllowedCloneSourceNamespaces = %#v, want %#v", policy.AllowedCloneSourceNamespaces, want)
	}
	if want := []string{"2Mi", "1Gi"}; !reflect.DeepEqual(policy.AllowedHugepagesSizes, want) {
		t.Fatalf("AllowedHugepagesSizes = %#v, want %#v", policy.AllowedHugepagesSizes, want)
	}

	tpl, templateErr := client.Template.Query().
		Where(enttemplate.NameEQ(fx.TemplateName)).
		Only(ctx)
	if templateErr != nil {
		t.Fatalf("query template: %v", templateErr)
	}
	if tpl.SourceType != "cdi_image_import" || tpl.ImageURL != defaultTemplateImageURL || tpl.PvcName != "" || tpl.PvcNamespace != "" || !tpl.Enabled {
		t.Fatalf("template after rerun = source=%q image=%q pvc=%q/%q enabled=%v, want canonical live template", tpl.SourceType, tpl.ImageURL, tpl.PvcNamespace, tpl.PvcName, tpl.Enabled)
	}

	size, sizeErr := client.InstanceSize.Query().
		Where(entinstancesize.NameEQ(fx.SizeName)).
		Only(ctx)
	if sizeErr != nil {
		t.Fatalf("query instance size: %v", sizeErr)
	}
	if size.CPUCores != 1 || size.MemoryGi != 2 || size.CPURequest != 0.5 || !size.Enabled {
		t.Fatalf("instance size after rerun = cpu %.1f memory %.1f request %.1f enabled %v, want canonical small fixture", size.CPUCores, size.MemoryGi, size.CPURequest, size.Enabled)
	}
}

func TestE2ESeedResolveAPIManagedFixtureModes(t *testing.T) {
	t.Setenv("E2E_KUBECONFIG_B64", "")
	t.Setenv("E2E_KUBECONFIG_PATH", "")

	ctx := context.Background()
	client := testutil.OpenEntPostgres(t, "e2e_seed_resolve_modes")
	fx := testFixtureConfig()

	if err := ensureNamespaceRegistry(ctx, client, fx); err != nil {
		t.Fatalf("ensureNamespaceRegistry() error = %v", err)
	}
	clusterID, clusterErr := resolveClusterID(ctx, client, fx, false)
	if clusterErr != nil {
		t.Fatalf("resolveClusterID(create mode) error = %v", clusterErr)
	}
	systemID, systemErr := resolveSystemID(ctx, client, fx, false)
	if systemErr != nil {
		t.Fatalf("resolveSystemID(create mode) error = %v", systemErr)
	}
	serviceID, serviceErr := resolveServiceID(ctx, client, fx, systemID, false)
	if serviceErr != nil {
		t.Fatalf("resolveServiceID(create mode) error = %v", serviceErr)
	}

	if err := ensureExistingNamespaceRegistry(ctx, client, fx.NamespaceName); err != nil {
		t.Fatalf("ensureExistingNamespaceRegistry() error = %v", err)
	}
	gotClusterID, gotClusterErr := resolveClusterID(ctx, client, fx, true)
	if gotClusterErr != nil {
		t.Fatalf("resolveClusterID(skip mode) error = %v", gotClusterErr)
	}
	if gotClusterID != clusterID {
		t.Fatalf("resolveClusterID(skip mode) = %q, want %q", gotClusterID, clusterID)
	}
	gotSystemID, gotSystemErr := resolveSystemID(ctx, client, fx, true)
	if gotSystemErr != nil {
		t.Fatalf("resolveSystemID(skip mode) error = %v", gotSystemErr)
	}
	if gotSystemID != systemID {
		t.Fatalf("resolveSystemID(skip mode) = %q, want %q", gotSystemID, systemID)
	}
	gotServiceID, gotServiceErr := resolveServiceID(ctx, client, fx, systemID, true)
	if gotServiceErr != nil {
		t.Fatalf("resolveServiceID(skip mode) error = %v", gotServiceErr)
	}
	if gotServiceID != serviceID {
		t.Fatalf("resolveServiceID(skip mode) = %q, want %q", gotServiceID, serviceID)
	}

	if err := ensureExistingNamespaceRegistry(ctx, client, "missing-ns"); err == nil || !strings.Contains(err.Error(), "namespace \"missing-ns\" not found") {
		t.Fatalf("ensureExistingNamespaceRegistry(missing) error = %v, want missing namespace diagnostic", err)
	}
	missingClusterFx := fx
	missingClusterFx.ClusterName = "missing-cluster"
	if _, err := resolveClusterID(ctx, client, missingClusterFx, true); err == nil || !strings.Contains(err.Error(), "cluster \"missing-cluster\" not found") {
		t.Fatalf("resolveClusterID(missing skip mode) error = %v, want missing cluster diagnostic", err)
	}
	missingSystemFx := fx
	missingSystemFx.SystemName = "missing-sys"
	if _, err := resolveSystemID(ctx, client, missingSystemFx, true); err == nil || !strings.Contains(err.Error(), "system \"missing-sys\" not found") {
		t.Fatalf("resolveSystemID(missing skip mode) error = %v, want missing system diagnostic", err)
	}
	missingServiceFx := fx
	missingServiceFx.ServiceName = "missing-svc"
	if _, err := resolveServiceID(ctx, client, missingServiceFx, systemID, true); err == nil || !strings.Contains(err.Error(), "service \"missing-svc\"") {
		t.Fatalf("resolveServiceID(missing skip mode) error = %v, want missing service diagnostic", err)
	}
}

func TestE2ESeedExtendedFixturesAreIdempotent(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenEntPostgres(t, "e2e_seed_extended")
	fx := testFixtureConfig()

	if err := ensureAuthProvider(ctx, client); err != nil {
		t.Fatalf("ensureAuthProvider() initial error = %v", err)
	}
	provider, providerErr := client.AuthProvider.Query().
		Where(entauthprovider.NameEQ(defaultAuthProviderName)).
		Only(ctx)
	if providerErr != nil {
		t.Fatalf("query auth provider: %v", providerErr)
	}
	if provider.AuthType != "ldap" || !provider.Enabled || provider.Config["host"] != "ldap.e2e.invalid" {
		t.Fatalf("auth provider = type %q enabled %v config %#v, want enabled ldap fixture", provider.AuthType, provider.Enabled, provider.Config)
	}
	if _, err := client.AuthProvider.UpdateOneID(provider.ID).
		SetConfig(map[string]interface{}{"host": "stale.invalid"}).
		SetEnabled(false).
		Save(ctx); err != nil {
		t.Fatalf("mutate auth provider: %v", err)
	}
	if err := ensureAuthProvider(ctx, client); err != nil {
		t.Fatalf("ensureAuthProvider() rerun error = %v", err)
	}
	provider, providerErr = client.AuthProvider.Query().
		Where(entauthprovider.NameEQ(defaultAuthProviderName)).
		Only(ctx)
	if providerErr != nil {
		t.Fatalf("query auth provider after rerun: %v", providerErr)
	}
	if !provider.Enabled || provider.Config["host"] != "ldap.e2e.invalid" {
		t.Fatalf("auth provider after rerun = enabled %v config %#v, want reset fixture", provider.Enabled, provider.Config)
	}

	secondUserID, secondUserErr := ensureSecondUser(ctx, client, fx)
	if secondUserErr != nil {
		t.Fatalf("ensureSecondUser() initial error = %v", secondUserErr)
	}
	if _, err := client.User.UpdateOneID(secondUserID).
		SetEmail("stale@example.com").
		SetDisplayName("Stale User").
		SetEnabled(false).
		Save(ctx); err != nil {
		t.Fatalf("mutate second user: %v", err)
	}
	rerunSecondUserID, rerunSecondUserErr := ensureSecondUser(ctx, client, fx)
	if rerunSecondUserErr != nil {
		t.Fatalf("ensureSecondUser() rerun error = %v", rerunSecondUserErr)
	}
	if rerunSecondUserID != secondUserID {
		t.Fatalf("ensureSecondUser() rerun ID = %q, want %q", rerunSecondUserID, secondUserID)
	}
	secondUser, secondUserQueryErr := client.User.Query().
		Where(entuser.UsernameEQ(fx.SecondUsername)).
		Only(ctx)
	if secondUserQueryErr != nil {
		t.Fatalf("query second user: %v", secondUserQueryErr)
	}
	if secondUser.Email != fx.SecondEmail || secondUser.DisplayName != fx.SecondDisplayName || !secondUser.Enabled {
		t.Fatalf("second user after rerun = email %q display %q enabled %v, want fixture values", secondUser.Email, secondUser.DisplayName, secondUser.Enabled)
	}

	if err := ensureApprovalTickets(ctx, client); err != nil {
		t.Fatalf("ensureApprovalTickets() initial error = %v", err)
	}
	ticketIDs := []string{"tkt-e2e-seed-approve", "tkt-e2e-seed-reject", "tkt-e2e-seed-cancel"}
	ticketCount, ticketCountErr := client.Ticket.Query().
		Where(entticket.IDIn(ticketIDs...)).
		Count(ctx)
	if ticketCountErr != nil {
		t.Fatalf("count approval tickets: %v", ticketCountErr)
	}
	if ticketCount != len(ticketIDs) {
		t.Fatalf("approval ticket count = %d, want %d", ticketCount, len(ticketIDs))
	}
	if _, err := client.Ticket.UpdateOneID("tkt-e2e-seed-approve").
		SetStatus(entticket.StatusAPPROVED).
		SetApprover("previous-admin").
		SetRejectReason("stale reason").
		Save(ctx); err != nil {
		t.Fatalf("mutate approval ticket: %v", err)
	}
	if err := ensureApprovalTickets(ctx, client); err != nil {
		t.Fatalf("ensureApprovalTickets() rerun error = %v", err)
	}
	for _, id := range ticketIDs {
		ticket, ticketErr := client.Ticket.Get(ctx, id)
		if ticketErr != nil {
			t.Fatalf("query approval ticket %s: %v", id, ticketErr)
		}
		if ticket.Status != entticket.StatusPENDING || ticket.Approver != "" || ticket.RejectReason != "" {
			t.Fatalf("ticket %s after rerun = status %s approver %q reject %q, want clean PENDING", id, ticket.Status, ticket.Approver, ticket.RejectReason)
		}
	}
	eventCount, eventCountErr := client.DomainEvent.Query().
		Where(entdomainevent.IDIn("evt-e2e-seed-approve", "evt-e2e-seed-reject", "evt-e2e-seed-cancel")).
		Count(ctx)
	if eventCountErr != nil {
		t.Fatalf("count approval domain events: %v", eventCountErr)
	}
	if eventCount != len(ticketIDs) {
		t.Fatalf("approval event count = %d, want %d", eventCount, len(ticketIDs))
	}
}

func TestE2ESeedRoleBindingsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenEntPostgres(t, "e2e_seed_role_bindings")
	fx := testFixtureConfig()

	platformRoleID := seedE2ERole(ctx, t, client, "role-platform-admin", "PlatformAdmin", []string{"platform:admin"})
	seedE2ERole(ctx, t, client, "role-test-engineer", "TestEngineer", []string{"vm:read"})

	adminID, adminErr := ensureAdminUser(ctx, client, fx)
	if adminErr != nil {
		t.Fatalf("ensureAdminUser() error = %v", adminErr)
	}
	if err := ensureAdminRoleBinding(ctx, client, adminID); err != nil {
		t.Fatalf("ensureAdminRoleBinding() initial error = %v", err)
	}
	if err := ensureAdminRoleBinding(ctx, client, adminID); err != nil {
		t.Fatalf("ensureAdminRoleBinding() rerun error = %v", err)
	}

	adminBindingCount, adminBindingCountErr := client.RoleBinding.Query().
		Where(
			entrolebinding.HasUserWith(entuser.IDEQ(adminID)),
			entrolebinding.HasRoleWith(entrole.IDEQ(platformRoleID)),
			entrolebinding.ScopeTypeEQ("global"),
		).
		Count(ctx)
	if adminBindingCountErr != nil {
		t.Fatalf("count admin global role bindings: %v", adminBindingCountErr)
	}
	if adminBindingCount != 1 {
		t.Fatalf("admin global role binding count = %d, want 1", adminBindingCount)
	}

	if err := ensureGlobalRoleBinding(ctx, client, adminID, "MissingRole", seedActor); err == nil || !strings.Contains(err.Error(), "MissingRole role not found") {
		t.Fatalf("ensureGlobalRoleBinding(missing role) error = %v, want missing role diagnostic", err)
	}

	secondUserID, secondUserErr := ensureSecondUser(ctx, client, fx)
	if secondUserErr != nil {
		t.Fatalf("ensureSecondUser() error = %v", secondUserErr)
	}
	systemID, systemErr := ensureSystem(ctx, client, fx)
	if systemErr != nil {
		t.Fatalf("ensureSystem() error = %v", systemErr)
	}

	if err := ensureSystemMemberBinding(ctx, client, secondUserID, systemID, entresourcerolebinding.RoleMember); err != nil {
		t.Fatalf("ensureSystemMemberBinding(member) error = %v", err)
	}
	if err := ensureSystemMemberBinding(ctx, client, secondUserID, systemID, entresourcerolebinding.RoleAdmin); err != nil {
		t.Fatalf("ensureSystemMemberBinding(admin rerun) error = %v", err)
	}
	memberBinding, memberBindingErr := client.ResourceRoleBinding.Query().
		Where(
			entresourcerolebinding.UserIDEQ(secondUserID),
			entresourcerolebinding.ResourceTypeEQ("system"),
			entresourcerolebinding.ResourceIDEQ(systemID),
		).
		Only(ctx)
	if memberBindingErr != nil {
		t.Fatalf("query system member binding: %v", memberBindingErr)
	}
	if memberBinding.Role != entresourcerolebinding.RoleAdmin {
		t.Fatalf("system member binding role = %s, want %s after rerun update", memberBinding.Role, entresourcerolebinding.RoleAdmin)
	}
	memberBindingCount, memberBindingCountErr := client.ResourceRoleBinding.Query().
		Where(
			entresourcerolebinding.UserIDEQ(secondUserID),
			entresourcerolebinding.ResourceTypeEQ("system"),
			entresourcerolebinding.ResourceIDEQ(systemID),
		).
		Count(ctx)
	if memberBindingCountErr != nil {
		t.Fatalf("count system member bindings: %v", memberBindingCountErr)
	}
	if memberBindingCount != 1 {
		t.Fatalf("system member binding count = %d, want 1", memberBindingCount)
	}
}

func TestEnsureNotificationsRecreatesAndResetsAllSeedNotifications(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenEntPostgres(t, "e2e_seed_notifications")
	fx := testFixtureConfig()

	adminID, adminErr := ensureAdminUser(ctx, client, fx)
	if adminErr != nil {
		t.Fatalf("ensureAdminUser() error = %v", adminErr)
	}
	if err := ensureNotifications(ctx, client, adminID); err != nil {
		t.Fatalf("ensureNotifications() initial error = %v", err)
	}
	if _, err := client.Notification.UpdateOneID("notif-e2e-seed-01").
		SetRead(true).
		Save(ctx); err != nil {
		t.Fatalf("mark first notification read: %v", err)
	}
	if err := client.Notification.DeleteOneID("notif-e2e-seed-02").Exec(ctx); err != nil {
		t.Fatalf("delete second notification: %v", err)
	}

	if err := ensureNotifications(ctx, client, adminID); err != nil {
		t.Fatalf("ensureNotifications() rerun error = %v", err)
	}
	items, itemsErr := client.Notification.Query().
		Where(entnotification.IDIn("notif-e2e-seed-01", "notif-e2e-seed-02")).
		All(ctx)
	if itemsErr != nil {
		t.Fatalf("query seed notifications: %v", itemsErr)
	}
	if len(items) != 2 {
		t.Fatalf("seed notifications after rerun = %d, want 2", len(items))
	}
	for _, item := range items {
		if item.Read {
			t.Fatalf("%s Read = true, want false after rerun reset", item.ID)
		}
		if item.QueryUser().OnlyX(ctx).ID != adminID {
			t.Fatalf("%s user ID mismatch", item.ID)
		}
	}
}

func TestEnsureBatchApprovalTicketsRebuildsDeterministicFixtures(t *testing.T) {
	t.Setenv("E2E_KUBECONFIG_B64", "")
	t.Setenv("E2E_KUBECONFIG_PATH", "")

	ctx := context.Background()
	client := testutil.OpenEntPostgres(t, "e2e_seed_batch")
	fx := testFixtureConfig()

	clusterID, clusterErr := ensureCluster(ctx, client, fx)
	if clusterErr != nil {
		t.Fatalf("ensureCluster() error = %v", clusterErr)
	}
	systemID, systemErr := ensureSystem(ctx, client, fx)
	if systemErr != nil {
		t.Fatalf("ensureSystem() error = %v", systemErr)
	}
	if _, err := ensureService(ctx, client, fx, systemID); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	if err := ensureTemplate(ctx, client, fx); err != nil {
		t.Fatalf("ensureTemplate() error = %v", err)
	}
	if err := ensureInstanceSize(ctx, client, fx); err != nil {
		t.Fatalf("ensureInstanceSize() error = %v", err)
	}
	if err := ensureBatchApprovalTickets(ctx, client, fx.AdminUsername, clusterID, fx.NamespaceName); err != nil {
		t.Fatalf("ensureBatchApprovalTickets() initial error = %v", err)
	}
	assertBatchProjection(ctx, t, client, "batch-e2e-seed-failed", entbatchticket.StatusFAILED, 3, 1, 2, 0)
	assertBatchProjection(ctx, t, client, "batch-e2e-seed-pending", entbatchticket.StatusIN_PROGRESS, 5, 0, 0, 5)

	parent, parentErr := client.Ticket.Get(ctx, "batch-e2e-seed-failed")
	if parentErr != nil {
		t.Fatalf("query failed batch parent ticket: %v", parentErr)
	}
	if parent.SelectedClusterID != clusterID {
		t.Fatalf("failed batch selected cluster = %q, want %q", parent.SelectedClusterID, clusterID)
	}
	parentEvent, parentEventErr := client.DomainEvent.Get(ctx, "evt-batch-e2e-seed-failed-parent")
	if parentEventErr != nil {
		t.Fatalf("query failed batch parent event: %v", parentEventErr)
	}
	if parentEvent.Status != entdomainevent.StatusFAILED {
		t.Fatalf("failed batch parent event status = %s, want FAILED", parentEvent.Status)
	}

	if _, err := client.Ticket.Update().
		Where(entticket.ParentTicketIDEQ("batch-e2e-seed-failed")).
		SetStatus(entticket.StatusSUCCESS).
		Save(ctx); err != nil {
		t.Fatalf("mutate failed batch child tickets: %v", err)
	}
	if _, err := client.BatchTicket.UpdateOneID("batch-e2e-seed-failed").
		SetChildCount(99).
		SetSuccessCount(99).
		SetFailedCount(0).
		SetPendingCount(0).
		Save(ctx); err != nil {
		t.Fatalf("mutate failed batch projection: %v", err)
	}
	if err := ensureBatchApprovalTickets(ctx, client, fx.AdminUsername, clusterID, fx.NamespaceName); err != nil {
		t.Fatalf("ensureBatchApprovalTickets() rerun error = %v", err)
	}
	assertBatchProjection(ctx, t, client, "batch-e2e-seed-failed", entbatchticket.StatusFAILED, 3, 1, 2, 0)
}

func testFixtureConfig() fixtureConfig {
	return fixtureConfig{
		AdminUsername:     defaultAdminUsername,
		AdminPassword:     defaultAdminPassword,
		AdminEmail:        defaultAdminEmail,
		SecondUsername:    defaultSecondUsername,
		SecondPassword:    defaultSecondPassword,
		SecondEmail:       defaultSecondEmail,
		SecondDisplayName: defaultSecondDisplayName,
		SecondRoleName:    defaultSecondRoleName,
		NamespaceName:     defaultNamespaceName,
		ClusterName:       defaultClusterName,
		ClusterAPIURL:     defaultClusterAPIURL,
		SystemName:        defaultSystemName,
		ServiceName:       defaultServiceName,
		TemplateName:      defaultTemplateName,
		SizeName:          defaultSizeName,
		RunningVMID:       defaultRunningVMID,
		StoppedVMID:       defaultStoppedVMID,
	}
}

func mustGetCluster(ctx context.Context, t *testing.T, client *ent.Client, clusterID string) *ent.Cluster {
	t.Helper()
	cluster, err := client.Cluster.Get(ctx, clusterID)
	if err != nil {
		t.Fatalf("query cluster %s: %v", clusterID, err)
	}
	return cluster
}

func seedE2ERole(ctx context.Context, t *testing.T, client *ent.Client, id, name string, permissions []string) string {
	t.Helper()
	_, err := client.Role.Create().
		SetID(id).
		SetName(name).
		SetDisplayName(name).
		SetDescription("seed test role").
		SetPermissions(permissions).
		SetBuiltIn(true).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role %s: %v", name, err)
	}
	return id
}

func assertBatchProjection(
	ctx context.Context,
	t *testing.T,
	client *ent.Client,
	id string,
	status entbatchticket.Status,
	childCount int,
	successCount int,
	failedCount int,
	pendingCount int,
) {
	t.Helper()
	projection, err := client.BatchTicket.Get(ctx, id)
	if err != nil {
		t.Fatalf("query batch projection %s: %v", id, err)
	}
	if projection.Status != status ||
		projection.ChildCount != childCount ||
		projection.SuccessCount != successCount ||
		projection.FailedCount != failedCount ||
		projection.PendingCount != pendingCount {
		t.Fatalf(
			"batch projection %s = status %s children/success/failed/pending %d/%d/%d/%d, want %s %d/%d/%d/%d",
			id,
			projection.Status,
			projection.ChildCount,
			projection.SuccessCount,
			projection.FailedCount,
			projection.PendingCount,
			status,
			childCount,
			successCount,
			failedCount,
			pendingCount,
		)
	}
	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(id)).
		All(ctx)
	if err != nil {
		t.Fatalf("query batch children %s: %v", id, err)
	}
	if len(children) != childCount {
		t.Fatalf("batch %s child tickets = %d, want %d", id, len(children), childCount)
	}
}
