package provider

import "context"

func (p *KubeVirtProviderImpl) ViolatingGetVM(ctx context.Context, client KubeVirtClusterClient) {
	_, _ = client.VM().Get(ctx, "team-a", "vm-a", GetOptions{})  // want "K8s operation VM\\(\\)\\.Get must use bounded operation-timeout context opCtx"
	_, _ = client.VMI().Get(ctx, "team-a", "vm-a", GetOptions{}) // want "K8s operation VMI\\(\\)\\.Get must use bounded operation-timeout context opCtx"
	_, _ = client.Nodes().Get(ctx, "node-a", GetOptions{})       // want "K8s operation Nodes\\(\\)\\.Get must use bounded operation-timeout context opCtx"
}

func (p *KubeVirtProviderImpl) ViolatingListVMs(ctx context.Context, client KubeVirtClusterClient) {
	_, _ = client.VM().List(ctx, "team-a", ListOptions{})  // want "K8s operation VM\\(\\)\\.List must use bounded operation-timeout context opCtx"
	_, _ = client.VMI().List(ctx, "team-a", ListOptions{}) // want "K8s operation VMI\\(\\)\\.List must use bounded operation-timeout context opCtx"
}

func (c *ClusterHealthChecker) CheckCluster(ctx context.Context, client KubeVirtClusterClient) {
	_, _ = client.KubeVirt().GetVersion(ctx)                // want "K8s operation KubeVirt\\(\\)\\.GetVersion must use bounded operation-timeout context opCtx"
	_, _ = c.capabilityDetector.Detect(ctx, client)         // want "K8s operation Detect must use bounded operation-timeout context opCtx"
	_, _ = detectStorageClasses(ctx, client.StorageClass()) // want "K8s operation detectStorageClasses must use bounded operation-timeout context opCtx"
}

func (d *CapabilityDetector) ViolatingCapability(ctx context.Context, client KubeVirtClusterClient) {
	_, _ = client.KubeVirt().GetFeatureGates(ctx)  // want "K8s operation KubeVirt\\(\\)\\.GetFeatureGates must use bounded operation-timeout context opCtx"
	_, _ = client.Nodes().List(ctx, ListOptions{}) // want "K8s operation Nodes\\(\\)\\.List must use bounded operation-timeout context opCtx"
}

func helperReceivesRawContext(ctx context.Context, nodes NodeClient, storageClient StorageClassClient) {
	_, _ = nodes.List(ctx, ListOptions{})         // want "K8s operation NodeClient\\.List must use bounded operation-timeout context opCtx"
	_, _ = storageClient.List(ctx, ListOptions{}) // want "K8s operation StorageClassClient\\.List must use bounded operation-timeout context opCtx"
}

func helperReadsInstanceTypesWithRawContext(ctx context.Context, catalog InstanceTypeCatalogClient) {
	_, _ = catalog.ListInstanceTypes(ctx, "team-a", ListOptions{}) // want "K8s operation InstanceTypeCatalogClient\\.ListInstanceTypes must use bounded operation-timeout context opCtx"
	_, _ = catalog.ListClusterInstanceTypes(ctx, ListOptions{})    // want "K8s operation InstanceTypeCatalogClient\\.ListClusterInstanceTypes must use bounded operation-timeout context opCtx"
	_, _ = catalog.ListPreferences(ctx, "team-a", ListOptions{})   // want "K8s operation InstanceTypeCatalogClient\\.ListPreferences must use bounded operation-timeout context opCtx"
	_, _ = catalog.ListClusterPreferences(ctx, ListOptions{})      // want "K8s operation InstanceTypeCatalogClient\\.ListClusterPreferences must use bounded operation-timeout context opCtx"
}
