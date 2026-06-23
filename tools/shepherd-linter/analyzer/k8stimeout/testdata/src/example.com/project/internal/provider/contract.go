package provider

import "context"

type KubeVirtProviderImpl struct{}

type ClusterHealthChecker struct {
	capabilityDetector *CapabilityDetector
}

type CapabilityDetector struct{}

func (*CapabilityDetector) Detect(context.Context, KubeVirtClusterClient) ([]string, error) {
	return nil, nil
}

type KubeVirtClusterClient interface {
	KubeVirt() KubeVirtCRClient
	Nodes() NodeClient
	StorageClass() StorageClassClient
	VM() VirtualMachineClient
	VMI() VirtualMachineInstanceClient
}

type KubeVirtCRClient interface {
	GetVersion(context.Context) (string, error)
	GetFeatureGates(context.Context) ([]string, error)
}

type VirtualMachineClient interface {
	Get(context.Context, string, string, GetOptions) (*VirtualMachine, error)
	List(context.Context, string, ListOptions) (*VirtualMachineList, error)
}

type VirtualMachineInstanceClient interface {
	Get(context.Context, string, string, GetOptions) (*VirtualMachineInstance, error)
	List(context.Context, string, ListOptions) (*VirtualMachineInstanceList, error)
}

type NodeClient interface {
	Get(context.Context, string, GetOptions) (*Node, error)
	List(context.Context, ListOptions) (*NodeList, error)
}

type StorageClassClient interface {
	List(context.Context, ListOptions) (*StorageClassList, error)
}

type InstanceTypeCatalogClient interface {
	ListInstanceTypes(context.Context, string, ListOptions) (*InstanceTypeList, error)
	ListClusterInstanceTypes(context.Context, ListOptions) (*ClusterInstanceTypeList, error)
	ListPreferences(context.Context, string, ListOptions) (*PreferenceList, error)
	ListClusterPreferences(context.Context, ListOptions) (*ClusterPreferenceList, error)
}

type GetOptions struct{}
type ListOptions struct{}
type ClusterInstanceTypeList struct{}
type ClusterPreferenceList struct{}
type InstanceTypeList struct{}
type Node struct{}
type NodeList struct{}
type PreferenceList struct{}
type StorageClassList struct{}
type VirtualMachine struct{}
type VirtualMachineInstance struct{}
type VirtualMachineInstanceList struct{}
type VirtualMachineList struct{}

func detectStorageClasses(context.Context, StorageClassClient) ([]string, error) {
	return nil, nil
}
