package app

import (
	"context"
	"slices"
	"testing"

	storagev1 "k8s.io/api/storage/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type stubLifecycleKVCRClient struct {
	version      string
	featureGates []string
}

func (s *stubLifecycleKVCRClient) GetFeatureGates(context.Context) ([]string, error) {
	return s.featureGates, nil
}

func (s *stubLifecycleKVCRClient) GetVersion(context.Context) (string, error) {
	return s.version, nil
}

type stubLifecycleStorageClassClient struct {
	list *storagev1.StorageClassList
}

func (s *stubLifecycleStorageClassClient) Get(context.Context, string, k8smetav1.GetOptions) (*storagev1.StorageClass, error) {
	return nil, nil
}

func (s *stubLifecycleStorageClassClient) List(context.Context, k8smetav1.ListOptions) (*storagev1.StorageClassList, error) {
	if s.list == nil {
		return &storagev1.StorageClassList{}, nil
	}
	return s.list, nil
}

type stubLifecycleClusterClient struct {
	kvCR         provider.KubeVirtCRClient
	storageClass provider.StorageClassClient
}

func (s *stubLifecycleClusterClient) VM() provider.VirtualMachineClient             { return nil }
func (s *stubLifecycleClusterClient) VMI() provider.VirtualMachineInstanceClient    { return nil }
func (s *stubLifecycleClusterClient) DataVolume() provider.DataVolumeClient         { return nil }
func (s *stubLifecycleClusterClient) StorageProfile() provider.StorageProfileClient { return nil }
func (s *stubLifecycleClusterClient) PVC() provider.PersistentVolumeClaimClient     { return nil }
func (s *stubLifecycleClusterClient) StorageClass() provider.StorageClassClient {
	return s.storageClass
}
func (s *stubLifecycleClusterClient) Events() provider.EventClient                { return nil }
func (s *stubLifecycleClusterClient) Namespaces() provider.NamespaceClient        { return nil }
func (s *stubLifecycleClusterClient) Pods() provider.PodClient                    { return nil }
func (s *stubLifecycleClusterClient) Authorization() provider.AuthorizationClient { return nil }
func (s *stubLifecycleClusterClient) SSA() provider.DynamicSSAClient              { return nil }
func (s *stubLifecycleClusterClient) KubeVirt() provider.KubeVirtCRClient         { return s.kvCR }

func TestApplication_refreshClusterHealth_PersistsDetectedCapabilities(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "app_lifecycle")
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
		return &stubLifecycleClusterClient{
			kvCR: &stubLifecycleKVCRClient{
				version:      "1.7.0",
				featureGates: []string{"Snapshot"},
			},
			storageClass: &stubLifecycleStorageClassClient{
				list: &storagev1.StorageClassList{
					Items: []storagev1.StorageClass{
						{ObjectMeta: k8smetav1.ObjectMeta{Name: "zeta"}},
						{ObjectMeta: k8smetav1.ObjectMeta{Name: "alpha"}},
					},
				},
			},
		}, nil
	}, 0)

	app := &Application{
		EntClient:   client,
		HealthCheck: checker,
	}

	refreshErr := app.refreshClusterHealth(ctx)
	if refreshErr != nil {
		t.Fatalf("refreshClusterHealth() error = %v", refreshErr)
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
	if got := stored.EnabledFeatures; !slices.Contains(got, "Snapshot") {
		t.Fatalf("enabled_features = %v, want Snapshot to be present", got)
	}
	if got := stored.StorageClasses; len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("storage_classes = %v, want [alpha zeta]", got)
	}
	if stored.StorageClassesUpdatedAt.IsZero() {
		t.Fatal("storage_classes_updated_at = zero, want populated timestamp")
	}
}
