package provider

import (
	"context"
	"fmt"
	"testing"

	storagev1 "k8s.io/api/storage/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func init() {
	_ = logger.Init("error", "json")
}

type stubHealthStorageClassClient struct {
	list *storagev1.StorageClassList
	err  error
}

func (s *stubHealthStorageClassClient) Get(_ context.Context, _ string, _ k8smetav1.GetOptions) (*storagev1.StorageClass, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubHealthStorageClassClient) List(_ context.Context, _ k8smetav1.ListOptions) (*storagev1.StorageClassList, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.list == nil {
		return &storagev1.StorageClassList{}, nil
	}
	return s.list, nil
}

type stubHealthClusterClient struct {
	kvCR         KubeVirtCRClient
	storageClass StorageClassClient
}

func (s *stubHealthClusterClient) VM() VirtualMachineClient             { return nil }
func (s *stubHealthClusterClient) VMI() VirtualMachineInstanceClient    { return nil }
func (s *stubHealthClusterClient) DataVolume() DataVolumeClient         { return nil }
func (s *stubHealthClusterClient) StorageProfile() StorageProfileClient { return nil }
func (s *stubHealthClusterClient) PVC() PersistentVolumeClaimClient     { return nil }
func (s *stubHealthClusterClient) StorageClass() StorageClassClient     { return s.storageClass }
func (s *stubHealthClusterClient) Events() EventClient                  { return nil }
func (s *stubHealthClusterClient) Pods() PodClient                      { return nil }
func (s *stubHealthClusterClient) Authorization() AuthorizationClient   { return nil }
func (s *stubHealthClusterClient) SSA() DynamicSSAClient                { return nil }
func (s *stubHealthClusterClient) KubeVirt() KubeVirtCRClient           { return s.kvCR }

func TestClusterHealthChecker_CheckCluster_DetectsStorageClasses(t *testing.T) {
	t.Parallel()

	checker := NewClusterHealthChecker(func(_ string) (KubeVirtClusterClient, error) {
		return &stubHealthClusterClient{
			kvCR: &stubKVCRClient{version: "1.7.0"},
			storageClass: &stubHealthStorageClassClient{
				list: &storagev1.StorageClassList{
					Items: []storagev1.StorageClass{
						{ObjectMeta: k8smetav1.ObjectMeta{Name: "zeta"}},
						{ObjectMeta: k8smetav1.ObjectMeta{Name: "alpha"}},
					},
				},
			},
		}, nil
	}, 0)

	health := checker.CheckCluster(context.Background(), "cluster-a")

	if health.Status != ClusterStatusHealthy {
		t.Fatalf("status = %s, want %s", health.Status, ClusterStatusHealthy)
	}
	if !health.StorageClassesDetected {
		t.Fatal("expected StorageClassesDetected=true")
	}
	if got, want := health.StorageClasses, []string{"alpha", "zeta"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("storage classes = %v, want %v", got, want)
	}
}

func TestClusterHealthChecker_CheckCluster_StorageClassDetectionGracefullyDegrades(t *testing.T) {
	t.Parallel()

	checker := NewClusterHealthChecker(func(_ string) (KubeVirtClusterClient, error) {
		return &stubHealthClusterClient{
			kvCR: &stubKVCRClient{version: "1.7.0"},
			storageClass: &stubHealthStorageClassClient{
				err: fmt.Errorf("forbidden"),
			},
		}, nil
	}, 0)

	health := checker.CheckCluster(context.Background(), "cluster-a")

	if health.Status != ClusterStatusHealthy {
		t.Fatalf("status = %s, want %s", health.Status, ClusterStatusHealthy)
	}
	if health.StorageClassesDetected {
		t.Fatal("expected StorageClassesDetected=false on degraded detection")
	}
	if len(health.StorageClasses) != 0 {
		t.Fatalf("storage classes = %v, want empty slice", health.StorageClasses)
	}
}
