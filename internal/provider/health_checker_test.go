package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func init() {
	_ = logger.Init("error", "json")
}

type stubHealthStorageClassClient struct {
	list    *storagev1.StorageClassList
	err     error
	listCtx context.Context
}

func (s *stubHealthStorageClassClient) Get(_ context.Context, _ string, _ k8smetav1.GetOptions) (*storagev1.StorageClass, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubHealthStorageClassClient) List(ctx context.Context, _ k8smetav1.ListOptions) (*storagev1.StorageClassList, error) {
	s.listCtx = ctx
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
func (s *stubHealthClusterClient) Namespaces() NamespaceClient          { return nil }
func (s *stubHealthClusterClient) Nodes() NodeClient                    { return nil }
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

type capturingHealthKVCRClient struct {
	version     string
	versionCtxs []context.Context
	gatesCtxs   []context.Context
}

func (s *capturingHealthKVCRClient) GetFeatureGates(ctx context.Context) ([]string, error) {
	s.gatesCtxs = append(s.gatesCtxs, ctx)
	return nil, nil
}

func (s *capturingHealthKVCRClient) GetVersion(ctx context.Context) (string, error) {
	s.versionCtxs = append(s.versionCtxs, ctx)
	return s.version, nil
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

func TestClusterHealthChecker_CheckClusterUsesOperationTimeoutForK8sProbes(t *testing.T) {
	t.Parallel()

	kvCR := &capturingHealthKVCRClient{version: "1.7.0"}
	storageClass := &stubHealthStorageClassClient{
		list: &storagev1.StorageClassList{},
	}
	checker := NewClusterHealthCheckerWithTimeout(func(_ string) (KubeVirtClusterClient, error) {
		return &stubHealthClusterClient{
			kvCR:         kvCR,
			storageClass: storageClass,
		}, nil
	}, time.Minute, 2*time.Second)

	health := checker.CheckCluster(context.Background(), "cluster-a")
	if health.Status != ClusterStatusHealthy {
		t.Fatalf("status = %s, want %s", health.Status, ClusterStatusHealthy)
	}

	if len(kvCR.versionCtxs) != 2 {
		t.Fatalf("version contexts = %d, want 2", len(kvCR.versionCtxs))
	}
	for _, captured := range kvCR.versionCtxs {
		assertContextDeadlineWithin(captured, t)
	}
	if len(kvCR.gatesCtxs) != 1 {
		t.Fatalf("feature-gates contexts = %d, want 1", len(kvCR.gatesCtxs))
	}
	assertContextDeadlineWithin(kvCR.gatesCtxs[0], t)
	assertContextDeadlineWithin(storageClass.listCtx, t)
}
