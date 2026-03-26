package provider

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubCachedClusterClient struct {
	id string
}

func (s *stubCachedClusterClient) VM() VirtualMachineClient             { return nil }
func (s *stubCachedClusterClient) VMI() VirtualMachineInstanceClient    { return nil }
func (s *stubCachedClusterClient) DataVolume() DataVolumeClient         { return nil }
func (s *stubCachedClusterClient) StorageProfile() StorageProfileClient { return nil }
func (s *stubCachedClusterClient) PVC() PersistentVolumeClaimClient     { return nil }
func (s *stubCachedClusterClient) StorageClass() StorageClassClient     { return nil }
func (s *stubCachedClusterClient) Events() EventClient                  { return nil }
func (s *stubCachedClusterClient) Namespaces() NamespaceClient          { return nil }
func (s *stubCachedClusterClient) Nodes() NodeClient                    { return nil }
func (s *stubCachedClusterClient) Pods() PodClient                      { return nil }
func (s *stubCachedClusterClient) Authorization() AuthorizationClient   { return nil }
func (s *stubCachedClusterClient) SSA() DynamicSSAClient                { return nil }
func (s *stubCachedClusterClient) KubeVirt() KubeVirtCRClient           { return nil }

func TestKubeconfigClusterFactory_ReusesClientUntilKubeconfigChanges(t *testing.T) {
	t.Parallel()

	loads := [][]byte{
		[]byte("cluster-a-v1"),
		[]byte("cluster-a-v1"),
		[]byte("cluster-a-v2"),
	}
	loadIndex := 0
	buildCount := 0

	factory := &kubeconfigClusterFactory{
		loader: func(cluster string) ([]byte, error) {
			require.Equal(t, "cluster-a", cluster)
			cfg := loads[loadIndex]
			loadIndex++
			return cfg, nil
		},
		buildClient: func(cluster string, kubeconfig []byte) (KubeVirtClusterClient, error) {
			buildCount++
			return &stubCachedClusterClient{
				id: fmt.Sprintf("%s-%d", string(kubeconfig), buildCount),
			}, nil
		},
		cache: make(map[string]kubeconfigClusterCacheEntry),
	}

	first, err := factory.get("cluster-a")
	require.NoError(t, err)

	second, err := factory.get("cluster-a")
	require.NoError(t, err)

	third, err := factory.get("cluster-a")
	require.NoError(t, err)

	require.Same(t, first, second)
	require.NotSame(t, first, third)
	require.Equal(t, 2, buildCount)
}

func TestKubeconfigClusterFactory_EvictsCachedClientWhenLoaderStartsFailing(t *testing.T) {
	t.Parallel()

	loadIndex := 0
	buildCount := 0

	factory := &kubeconfigClusterFactory{
		loader: func(cluster string) ([]byte, error) {
			require.Equal(t, "cluster-a", cluster)
			loadIndex++
			switch loadIndex {
			case 1:
				return []byte("cluster-a-v1"), nil
			case 2:
				return nil, fmt.Errorf("cluster disabled")
			default:
				return []byte("cluster-a-v1"), nil
			}
		},
		buildClient: func(cluster string, kubeconfig []byte) (KubeVirtClusterClient, error) {
			buildCount++
			return &stubCachedClusterClient{
				id: fmt.Sprintf("%s-%d", string(kubeconfig), buildCount),
			}, nil
		},
		cache: make(map[string]kubeconfigClusterCacheEntry),
	}

	first, err := factory.get("cluster-a")
	require.NoError(t, err)

	_, err = factory.get("cluster-a")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cluster disabled")

	second, err := factory.get("cluster-a")
	require.NoError(t, err)

	require.NotSame(t, first, second)
	require.Equal(t, 2, buildCount)
}
