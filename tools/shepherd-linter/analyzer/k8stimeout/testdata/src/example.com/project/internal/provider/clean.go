package provider

import (
	"context"
	"time"
)

func (p *KubeVirtProviderImpl) CleanGetVM(ctx context.Context, client KubeVirtClusterClient) {
	opCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, _ = client.VM().Get(opCtx, "team-a", "vm-a", GetOptions{})
	_, _ = client.VMI().Get(opCtx, "team-a", "vm-a", GetOptions{})
	_, _ = client.Nodes().Get(opCtx, "node-a", GetOptions{})
}

func (c *ClusterHealthChecker) CleanCheckCluster(ctx context.Context, client KubeVirtClusterClient) {
	opCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, _ = client.KubeVirt().GetVersion(opCtx)
	_, _ = c.capabilityDetector.Detect(opCtx, client)
	_, _ = detectStorageClasses(opCtx, client.StorageClass())
}

func (d *CapabilityDetector) CleanCapability(ctx context.Context, client KubeVirtClusterClient) {
	opCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, _ = client.KubeVirt().GetFeatureGates(opCtx)
	_, _ = client.Nodes().List(opCtx, ListOptions{})
}

func helperReceivesBoundedContext(opCtx context.Context, nodes NodeClient, storageClient StorageClassClient) {
	_, _ = nodes.List(opCtx, ListOptions{})
	_, _ = storageClient.List(opCtx, ListOptions{})
}

func helperReadsInstanceTypesWithBoundedContext(opCtx context.Context, catalog InstanceTypeCatalogClient) {
	_, _ = catalog.ListInstanceTypes(opCtx, "team-a", ListOptions{})
	_, _ = catalog.ListClusterInstanceTypes(opCtx, ListOptions{})
	_, _ = catalog.ListPreferences(opCtx, "team-a", ListOptions{})
	_, _ = catalog.ListClusterPreferences(opCtx, ListOptions{})
}
