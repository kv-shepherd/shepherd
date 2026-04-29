package modules

import (
	"context"
	"slices"
	"testing"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type stubModulesKVCRClient struct {
	version      string
	featureGates []string
}

func (s *stubModulesKVCRClient) GetFeatureGates(context.Context) ([]string, error) {
	return s.featureGates, nil
}

func (s *stubModulesKVCRClient) GetVersion(context.Context) (string, error) {
	return s.version, nil
}

type stubModulesStorageClassClient struct {
	list *storagev1.StorageClassList
}

func (s *stubModulesStorageClassClient) Get(context.Context, string, k8smetav1.GetOptions) (*storagev1.StorageClass, error) {
	return nil, nil
}

func (s *stubModulesStorageClassClient) List(context.Context, k8smetav1.ListOptions) (*storagev1.StorageClassList, error) {
	if s.list == nil {
		return &storagev1.StorageClassList{}, nil
	}
	return s.list, nil
}

type stubModulesClusterClient struct {
	kvCR         provider.KubeVirtCRClient
	storageClass provider.StorageClassClient
}

func (s *stubModulesClusterClient) VM() provider.VirtualMachineClient             { return nil }
func (s *stubModulesClusterClient) VMI() provider.VirtualMachineInstanceClient    { return nil }
func (s *stubModulesClusterClient) DataVolume() provider.DataVolumeClient         { return nil }
func (s *stubModulesClusterClient) StorageProfile() provider.StorageProfileClient { return nil }
func (s *stubModulesClusterClient) PVC() provider.PersistentVolumeClaimClient     { return nil }
func (s *stubModulesClusterClient) StorageClass() provider.StorageClassClient     { return s.storageClass }
func (s *stubModulesClusterClient) Events() provider.EventClient                  { return nil }
func (s *stubModulesClusterClient) Namespaces() provider.NamespaceClient          { return nil }
func (s *stubModulesClusterClient) Nodes() provider.NodeClient                    { return nil }
func (s *stubModulesClusterClient) Pods() provider.PodClient                      { return nil }
func (s *stubModulesClusterClient) Authorization() provider.AuthorizationClient   { return nil }
func (s *stubModulesClusterClient) SSA() provider.DynamicSSAClient                { return nil }
func (s *stubModulesClusterClient) KubeVirt() provider.KubeVirtCRClient           { return s.kvCR }

func TestAdminModule_ContributeServerDeps_WiresClusterPolicy(t *testing.T) {
	t.Parallel()

	module := NewAdminModule(&Infrastructure{EntClient: &ent.Client{}})
	var deps handlers.ServerDeps

	module.ContributeServerDeps(&deps)

	if deps.ClusterPolicy == nil {
		t.Fatal("ClusterPolicy dependency was not contributed")
	}
}

func TestNewServerDeps_WiresRefreshClusterHealth(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "modules_admin")
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-1").
		SetName("cluster-a").
		SetDisplayName("Cluster A").
		SetAPIServerURL("https://cluster.example.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusUNKNOWN).
		SetEnabled(true).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	checker := provider.NewClusterHealthChecker(func(string) (provider.KubeVirtClusterClient, error) {
		return &stubModulesClusterClient{
			kvCR: &stubModulesKVCRClient{
				version:      "1.7.0",
				featureGates: []string{"Snapshot"},
			},
			storageClass: &stubModulesStorageClassClient{
				list: &storagev1.StorageClassList{
					Items: []storagev1.StorageClass{
						{ObjectMeta: k8smetav1.ObjectMeta{Name: "zeta"}},
						{ObjectMeta: k8smetav1.ObjectMeta{Name: "alpha"}},
					},
				},
			},
		}, nil
	}, 0)

	deps := NewServerDeps(&config.Config{
		Session: config.SessionConfig{Lifetime: 2},
		Security: config.SecurityConfig{
			SessionSecret:       "session-secret-1234567890123456789012",
			EncryptionKey:       "3031323334353637383961626364656630313233343536373839616263646566",
			JWTVerificationKeys: []string{"verify-a"},
		},
	}, &Infrastructure{
		EntClient:   client,
		HealthCheck: checker,
	}, nil)

	if deps.RefreshClusterHealth == nil {
		t.Fatal("RefreshClusterHealth dependency was not wired")
	}
	refreshErr := deps.RefreshClusterHealth(ctx, "cl-1")
	if refreshErr != nil {
		t.Fatalf("RefreshClusterHealth() error = %v", refreshErr)
	}

	stored, err := client.Cluster.Get(ctx, "cl-1")
	if err != nil {
		t.Fatalf("reload cluster: %v", err)
	}
	if stored.Status != entcluster.StatusHEALTHY {
		t.Fatalf("status = %s, want %s", stored.Status, entcluster.StatusHEALTHY)
	}
	if stored.KubevirtVersion != "1.7.0" {
		t.Fatalf("kubevirt_version = %q, want %q", stored.KubevirtVersion, "1.7.0")
	}
	if got := stored.StorageClasses; len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("storage_classes = %v, want [alpha zeta]", got)
	}
	if got := stored.EnabledFeatures; !slices.Contains(got, "Snapshot") {
		t.Fatalf("enabled_features = %v, want Snapshot to be present", got)
	}
	if stored.StorageClassesUpdatedAt.IsZero() {
		t.Fatal("storage_classes_updated_at = zero, want populated timestamp")
	}
}

func TestNewServerDeps_RefreshClusterHealth_DisabledClusterDoesNotStayHealthy(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "modules_admin_disabled")
	ctx := t.Context()
	probeCalls := 0

	_, err := client.Cluster.Create().
		SetID("cl-disabled").
		SetName("cluster-disabled").
		SetDisplayName("Cluster Disabled").
		SetAPIServerURL("https://cluster.example.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnabled(false).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	checker := provider.NewClusterHealthChecker(func(string) (provider.KubeVirtClusterClient, error) {
		probeCalls++
		return &stubModulesClusterClient{
			kvCR: &stubModulesKVCRClient{version: "1.7.0"},
		}, nil
	}, 0)

	deps := NewServerDeps(&config.Config{
		Session: config.SessionConfig{Lifetime: 2},
		Security: config.SecurityConfig{
			SessionSecret:       "session-secret-1234567890123456789012",
			EncryptionKey:       "3031323334353637383961626364656630313233343536373839616263646566",
			JWTVerificationKeys: []string{"verify-a"},
		},
	}, &Infrastructure{
		EntClient:   client,
		HealthCheck: checker,
	}, nil)

	if deps.RefreshClusterHealth == nil {
		t.Fatal("RefreshClusterHealth dependency was not wired")
	}
	refreshErr := deps.RefreshClusterHealth(ctx, "cl-disabled")
	if refreshErr != nil {
		t.Fatalf("RefreshClusterHealth() error = %v", refreshErr)
	}

	stored, err := client.Cluster.Get(ctx, "cl-disabled")
	if err != nil {
		t.Fatalf("reload cluster: %v", err)
	}
	if stored.Status != entcluster.StatusUNKNOWN {
		t.Fatalf("status = %s, want %s", stored.Status, entcluster.StatusUNKNOWN)
	}
	if probeCalls != 0 {
		t.Fatalf("probeCalls = %d, want 0", probeCalls)
	}
}

func TestNewServerDeps_WiresAuthSessionValidation(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "modules_auth_sessions_client")
	pool := testutil.OpenPGXPool(t, "modules_auth_sessions_pool")

	deps := NewServerDeps(&config.Config{
		Session: config.SessionConfig{Lifetime: time.Hour, Cookie: "shepherd_session"},
		Security: config.SecurityConfig{
			SessionSecret:       "session-secret-1234567890123456789012",
			EncryptionKey:       "3031323334353637383961626364656630313233343536373839616263646566",
			JWTVerificationKeys: []string{"verify-a"},
		},
	}, &Infrastructure{
		EntClient: client,
		Pool:      pool,
	}, nil)

	if deps.AuthSessions == nil {
		t.Fatal("AuthSessions dependency was not wired")
	}
	if deps.JWTCfg.RevocationChecker == nil {
		t.Fatal("JWTCfg.RevocationChecker was not wired")
	}
	if deps.JWTCfg.ClaimsValidator == nil {
		t.Fatal("JWTCfg.ClaimsValidator was not wired")
	}
}
