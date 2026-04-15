package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/yaml"

	"kv-shepherd.io/shepherd/internal/domain"
)

// KubeVirtProviderImpl implements KubeVirtProvider using our client abstraction.
// ADR-0001: Use official kubevirt.io/client-go client (bound at composition root).
// ADR-0004: Interface composition (implements InfrastructureProvider + sub-providers).
// ADR-0011: VM writes use Server-Side Apply via DynamicSSAClient.
type KubeVirtProviderImpl struct {
	clientFactory    ClusterClientFactory
	mapper           *KubeVirtMapper
	operationTimeout time.Duration // ISSUE-011: enforce K8s op timeout
}

// NewKubeVirtProvider creates a new KubeVirtProvider.
// clientFactory creates a cluster client for the specified cluster.
func NewKubeVirtProvider(clientFactory ClusterClientFactory, operationTimeout time.Duration) *KubeVirtProviderImpl {
	if operationTimeout <= 0 {
		operationTimeout = 5 * time.Minute // same default as config.go
	}
	return &KubeVirtProviderImpl{
		clientFactory:    clientFactory,
		mapper:           NewKubeVirtMapper(),
		operationTimeout: operationTimeout,
	}
}

// withTimeout wraps ctx with the configured K8s operation timeout.
func (p *KubeVirtProviderImpl) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, p.operationTimeout)
}

// Name returns the provider name.
func (p *KubeVirtProviderImpl) Name() string { return "kubevirt" }

// Type returns the provider type.
func (p *KubeVirtProviderImpl) Type() string { return "kubevirt" }

// EnsureNamespace idempotently creates the target namespace on the selected
// cluster when it does not already exist.
func (p *KubeVirtProviderImpl) EnsureNamespace(ctx context.Context, cluster, namespace string) error {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return fmt.Errorf("ensure namespace: namespace is required")
	}

	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	if _, getErr := client.Namespaces().Get(opCtx, namespace, k8smetav1.GetOptions{}); getErr == nil {
		return nil
	} else if !apierrors.IsNotFound(getErr) {
		return fmt.Errorf("get namespace %s on cluster %s: %w", namespace, cluster, getErr)
	}

	namespaceManifest := []byte(fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespace))
	_, err = client.SSA().ApplyClusterScopedYAML(opCtx, namespaceGVR, namespaceManifest)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("apply namespace %s on cluster %s: %w", namespace, cluster, err)
	}
	return nil
}

// GetVM retrieves a VM from the specified cluster.
func (p *KubeVirtProviderImpl) GetVM(ctx context.Context, cluster, namespace, name string) (*domain.VM, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	vm, err := client.VM().Get(ctx, namespace, name, k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get vm %s/%s: %w", namespace, name, err)
	}

	// Try to get VMI for status enrichment
	vmi, _ := client.VMI().Get(ctx, namespace, name, k8smetav1.GetOptions{})

	mapped, err := p.mapper.MapVM(vm, vmi)
	if err != nil {
		return nil, err
	}
	if mapped.NodeName != "" {
		if node, nodeErr := client.Nodes().Get(ctx, mapped.NodeName, k8smetav1.GetOptions{}); nodeErr == nil {
			mapped.HostIP = resolveNodePrimaryIP(node)
		}
	}
	return mapped, nil
}

func (p *KubeVirtProviderImpl) GetVMManifestYAML(ctx context.Context, cluster, namespace, name string) (string, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return "", fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	vm, err := client.VM().Get(opCtx, namespace, name, k8smetav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get vm manifest %s/%s: %w", namespace, name, err)
	}

	jsonData, err := json.Marshal(vm)
	if err != nil {
		return "", fmt.Errorf("marshal vm manifest json: %w", err)
	}

	var manifest map[string]interface{}
	unmarshalErr := json.Unmarshal(jsonData, &manifest)
	if unmarshalErr != nil {
		return "", fmt.Errorf("unmarshal vm manifest json: %w", unmarshalErr)
	}

	unstructured.RemoveNestedField(manifest, "metadata", "managedFields")

	normalizedJSON, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal normalized vm manifest json: %w", err)
	}

	yamlData, err := yaml.JSONToYAML(normalizedJSON)
	if err != nil {
		return "", fmt.Errorf("convert vm manifest to yaml: %w", err)
	}
	return string(yamlData), nil
}

// OpenVNCStream opens a raw VNC stream backed by the official KubeVirt client.
func (p *KubeVirtProviderImpl) OpenVNCStream(ctx context.Context, cluster, namespace, name string) (net.Conn, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	select {
	case <-opCtx.Done():
		return nil, opCtx.Err()
	default:
	}

	conn, err := client.VMI().VNC(namespace, name, true)
	if err != nil {
		return nil, fmt.Errorf("open vnc stream %s/%s: %w", namespace, name, err)
	}
	return conn, nil
}

// OpenSerialConsoleStream opens a raw serial console stream backed by the official KubeVirt client.
func (p *KubeVirtProviderImpl) OpenSerialConsoleStream(ctx context.Context, cluster, namespace, name string) (net.Conn, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	select {
	case <-opCtx.Done():
		return nil, opCtx.Err()
	default:
	}

	conn, err := client.VMI().SerialConsole(namespace, name, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("open serial console %s/%s: %w", namespace, name, err)
	}
	return conn, nil
}

func (p *KubeVirtProviderImpl) DryRunVMMutation(
	ctx context.Context,
	cluster, namespace, name string,
	mutation *domain.VMMutation,
) error {
	_, err := p.patchVM(ctx, cluster, namespace, name, mutation, true)
	return err
}

func (p *KubeVirtProviderImpl) ExecuteVMMutation(
	ctx context.Context,
	cluster, namespace, name string,
	mutation *domain.VMMutation,
) (*domain.VM, error) {
	return p.patchVM(ctx, cluster, namespace, name, mutation, false)
}

// ListVMs lists VMs in the specified namespace.
func (p *KubeVirtProviderImpl) ListVMs(ctx context.Context, cluster, namespace string, opts ListOptions) (*domain.VMList, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	listOpts := k8smetav1.ListOptions{}
	if opts.LabelSelector != "" {
		listOpts.LabelSelector = opts.LabelSelector
	}
	if opts.FieldSelector != "" {
		listOpts.FieldSelector = opts.FieldSelector
	}
	if opts.Limit > 0 {
		listOpts.Limit = int64(opts.Limit)
	}
	if opts.Continue != "" {
		listOpts.Continue = opts.Continue
	}
	// ADR-0038: Route through K8s watch cache when ResourceVersion is available.
	// Explicitly assign even when empty string (baseline read).
	listOpts.ResourceVersion = opts.ResourceVersion
	// Kubernetes API best-practice: when specifying resourceVersion on LIST,
	// also set resourceVersionMatch for deterministic cache semantics.
	if opts.ResourceVersion != "" {
		listOpts.ResourceVersionMatch = k8smetav1.ResourceVersionMatchNotOlderThan
	}

	vmList, err := client.VM().List(ctx, namespace, listOpts)
	if err != nil {
		return nil, fmt.Errorf("list vms in %s: %w", namespace, err)
	}

	var vmis []kubevirtv1.VirtualMachineInstance
	// Batch fetch VMIs for status enrichment unless caller explicitly skips it.
	if !opts.SkipVMIEnrichment {
		vmiList, _ := client.VMI().List(ctx, namespace, k8smetav1.ListOptions{})
		if vmiList != nil {
			vmis = vmiList.Items
		}
	}

	result, err := p.mapper.MapVMList(vmList.Items, vmis)
	if err != nil {
		return nil, fmt.Errorf("map vm list: %w", err)
	}
	p.enrichVMListHostPlacement(ctx, client, result)

	if vmList.Continue != "" {
		result.Continue = vmList.Continue
	}

	return result, nil
}

func (p *KubeVirtProviderImpl) enrichVMListHostPlacement(ctx context.Context, client KubeVirtClusterClient, list *domain.VMList) {
	if list == nil || len(list.Items) == 0 {
		return
	}
	hostIPByNode := make(map[string]string)
	for _, item := range list.Items {
		if item == nil || item.NodeName == "" {
			continue
		}
		hostIP, ok := hostIPByNode[item.NodeName]
		if !ok {
			node, err := client.Nodes().Get(ctx, item.NodeName, k8smetav1.GetOptions{})
			if err != nil {
				hostIPByNode[item.NodeName] = ""
				continue
			}
			hostIP = resolveNodePrimaryIP(node)
			hostIPByNode[item.NodeName] = hostIP
		}
		item.HostIP = hostIP
	}
}

func resolveNodePrimaryIP(node *corev1.Node) string {
	if node == nil {
		return ""
	}
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP && strings.TrimSpace(address.Address) != "" {
			return strings.TrimSpace(address.Address)
		}
	}
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeExternalIP && strings.TrimSpace(address.Address) != "" {
			return strings.TrimSpace(address.Address)
		}
	}
	return ""
}

// CreateVM creates a VM via SSA Apply (ADR-0011).
//
// The provider acts as a "YAML porter" — it submits the rendered YAML as an
// SSA Patch, never constructing typed structs.
func (p *KubeVirtProviderImpl) CreateVM(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.VM, error) {
	if spec == nil {
		return nil, fmt.Errorf("create vm: spec is nil")
	}
	if strings.TrimSpace(spec.RenderedYAML) == "" {
		return nil, fmt.Errorf("create vm: spec.rendered_yaml is required (ADR-0011)")
	}

	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	if validateErr := validateYAMLResourceHalfSteps([]byte(spec.RenderedYAML)); validateErr != nil {
		return nil, fmt.Errorf("validate vm yaml resource steps for create: %w", validateErr)
	}

	// SSA Apply: idempotent, conflict-free, FieldOwner-tracked.
	result, err := client.SSA().ApplyYAML(opCtx, namespace, []byte(spec.RenderedYAML))
	if err != nil {
		return nil, fmt.Errorf("create vm %s/%s via ssa: %w", namespace, spec.Name, err)
	}

	// Read back the full typed object for domain mapping.
	created, err := client.VM().Get(opCtx, namespace, result.GetName(), k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get vm after ssa create: %w", err)
	}

	return p.mapper.MapVM(created, nil)
}

// UpdateVM updates a VM via SSA Apply (ADR-0011).
//
// Unlike the previous Get-Modify-Put pattern, SSA is declarative: the caller
// provides the full desired state in spec.RenderedYAML, and the API server
// merges it with existing state, preserving fields owned by other managers.
//
// Safety: The YAML metadata.name is validated against the `name` parameter
// to prevent accidental overwrites of a different VM.
func (p *KubeVirtProviderImpl) UpdateVM(ctx context.Context, cluster, namespace, name string, spec *domain.VMSpec) (*domain.VM, error) {
	if spec == nil {
		return nil, fmt.Errorf("update vm: spec is nil")
	}
	if strings.TrimSpace(spec.RenderedYAML) == "" {
		return nil, fmt.Errorf("update vm: spec.rendered_yaml is required (ADR-0011)")
	}

	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	updateManifest, err := enrichVMUpdateManifestWithCurrentDevices(
		opCtx,
		client,
		namespace,
		name,
		[]byte(spec.RenderedYAML),
	)
	if err != nil {
		return nil, fmt.Errorf("prepare vm update manifest: %w", err)
	}

	if validateErr := validateYAMLResourceHalfSteps(updateManifest); validateErr != nil {
		return nil, fmt.Errorf("validate vm yaml resource steps for update: %w", validateErr)
	}

	// Safety check: validate YAML target name matches the `name` parameter.
	yamlName, err := extractNameFromYAML(updateManifest)
	if err != nil {
		return nil, fmt.Errorf("validate yaml name for update: %w", err)
	}
	if yamlName != name {
		return nil, fmt.Errorf(
			"yaml metadata.name %q does not match update target %q: refusing to overwrite a different resource",
			yamlName, name,
		)
	}

	// SSA Apply is the same for create and update — naturally idempotent.
	result, err := client.SSA().ApplyYAML(opCtx, namespace, updateManifest)
	if err != nil {
		return nil, fmt.Errorf("update vm %s/%s via ssa: %w", namespace, name, err)
	}

	// Read back for domain mapping.
	updated, err := client.VM().Get(opCtx, namespace, result.GetName(), k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get vm after ssa update: %w", err)
	}

	return p.mapper.MapVM(updated, nil)
}

func (p *KubeVirtProviderImpl) patchVM(
	ctx context.Context,
	cluster, namespace, name string,
	mutation *domain.VMMutation,
	dryRun bool,
) (*domain.VM, error) {
	if mutation == nil {
		return nil, fmt.Errorf("patch vm: mutation is nil")
	}
	if mutation.Mode != domain.VMMutationModePatch {
		return nil, fmt.Errorf("patch vm: unsupported mutation mode %q", mutation.Mode)
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("patch vm: namespace is required")
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("patch vm: name is required")
	}
	if len(mutation.Payload) == 0 {
		return nil, fmt.Errorf("patch vm: payload is empty")
	}

	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	patchType, err := resolveVMMutationPatchType(mutation.PatchType)
	if err != nil {
		return nil, err
	}

	opts := k8smetav1.PatchOptions{}
	if dryRun {
		opts.DryRun = []string{k8smetav1.DryRunAll}
	}

	updated, err := client.VM().Patch(opCtx, namespace, name, patchType, mutation.Payload, opts)
	if err != nil {
		if dryRun {
			return nil, fmt.Errorf("dry-run patch vm %s/%s: %w", namespace, name, err)
		}
		return nil, fmt.Errorf("patch vm %s/%s: %w", namespace, name, err)
	}
	if dryRun {
		return nil, nil
	}
	return p.mapper.MapVM(updated, nil)
}

func enrichVMUpdateManifestWithCurrentDevices(
	ctx context.Context,
	client KubeVirtClusterClient,
	namespace, name string,
	yamlData []byte,
) ([]byte, error) {
	obj := &unstructured.Unstructured{}
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlData), 4096)
	if err := decoder.Decode(obj); err != nil {
		return nil, fmt.Errorf("decode update yaml: %w", err)
	}

	domainPatch, found, err := unstructured.NestedMap(
		obj.Object,
		"spec", "template", "spec", "domain",
	)
	if err != nil {
		return nil, fmt.Errorf("read update domain patch: %w", err)
	}
	if !found {
		return yamlData, nil
	}
	if _, hasDevices := domainPatch["devices"]; hasDevices {
		return yamlData, nil
	}

	currentVM, err := client.VM().Get(ctx, namespace, name, k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get current vm for domain defaults: %w", err)
	}
	if currentVM == nil || currentVM.Spec.Template == nil {
		return yamlData, nil
	}

	devicesMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(
		&currentVM.Spec.Template.Spec.Domain.Devices,
	)
	if err != nil {
		return nil, fmt.Errorf("convert current domain devices: %w", err)
	}
	if setErr := unstructured.SetNestedMap(
		obj.Object,
		devicesMap,
		"spec", "template", "spec", "domain", "devices",
	); setErr != nil {
		return nil, fmt.Errorf("set current domain devices on update patch: %w", setErr)
	}

	jsonData, err := json.Marshal(obj.Object)
	if err != nil {
		return nil, fmt.Errorf("marshal enriched update yaml: %w", err)
	}
	return jsonData, nil
}

func resolveVMMutationPatchType(patchType string) (types.PatchType, error) {
	switch strings.TrimSpace(patchType) {
	case "", domain.VMMutationPatchTypeMerge:
		return types.MergePatchType, nil
	case domain.VMMutationPatchTypeJSON:
		return types.JSONPatchType, nil
	default:
		return "", fmt.Errorf("patch vm: unsupported patch type %q", patchType)
	}
}

// DeleteVM deletes a VM.
func (p *KubeVirtProviderImpl) DeleteVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	return client.VM().Delete(opCtx, namespace, name, k8smetav1.DeleteOptions{})
}

// StartVM starts a stopped VM.
func (p *KubeVirtProviderImpl) StartVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VM().Start(opCtx, namespace, name, &kubevirtv1.StartOptions{})
}

// StopVM stops a running VM.
func (p *KubeVirtProviderImpl) StopVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VM().Stop(opCtx, namespace, name, &kubevirtv1.StopOptions{})
}

// RestartVM restarts a VM.
func (p *KubeVirtProviderImpl) RestartVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VM().Restart(opCtx, namespace, name, &kubevirtv1.RestartOptions{})
}

// PauseVM pauses a running VM.
func (p *KubeVirtProviderImpl) PauseVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VMI().Pause(opCtx, namespace, name, &kubevirtv1.PauseOptions{})
}

// UnpauseVM unpauses a paused VM.
func (p *KubeVirtProviderImpl) UnpauseVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VMI().Unpause(opCtx, namespace, name, &kubevirtv1.UnpauseOptions{})
}

// ValidateSpec performs dry-run validation via SSA DryRun (ADR-0011).
//
// Server-side DryRun is more authoritative than Go compiler checks for external
// CRDs: it validates against the actual CRD schema installed on the cluster.
func (p *KubeVirtProviderImpl) ValidateSpec(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.ValidationResult, error) {
	if spec == nil {
		return &domain.ValidationResult{
			Valid:  false,
			Errors: []string{"spec is nil"},
		}, nil
	}
	if strings.TrimSpace(spec.RenderedYAML) == "" {
		return &domain.ValidationResult{
			Valid:  false,
			Errors: []string{"spec.rendered_yaml is required (ADR-0011)"},
		}, nil
	}

	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	if err := validateYAMLResourceHalfSteps([]byte(spec.RenderedYAML)); err != nil {
		return &domain.ValidationResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("validate vm yaml resource steps: %v", err)},
		}, nil
	}

	dryRunErrMsg := ""
	if applyErr := client.SSA().DryRunApplyYAML(ctx, namespace, []byte(spec.RenderedYAML)); applyErr != nil {
		dryRunErrMsg = applyErr.Error()
	}
	if dryRunErrMsg != "" {
		return &domain.ValidationResult{
			Valid:  false,
			Errors: []string{dryRunErrMsg},
		}, nil
	}

	return &domain.ValidationResult{Valid: true}, nil
}

// GetDataVolume retrieves a CDI DataVolume for provisioning observability.
func (p *KubeVirtProviderImpl) GetDataVolume(ctx context.Context, cluster, namespace, name string) (*domain.DataVolume, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	dv, err := client.DataVolume().Get(opCtx, namespace, name, k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get datavolume %s/%s: %w", namespace, name, err)
	}

	conditions := make([]domain.ProvisioningCondition, 0, len(dv.Status.Conditions))
	for _, cond := range dv.Status.Conditions {
		conditions = append(conditions, domain.ProvisioningCondition{
			Type:               string(cond.Type),
			Status:             string(cond.Status),
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastTransitionTime: cond.LastTransitionTime.Time,
		})
	}

	return &domain.DataVolume{
		Name:         dv.Name,
		Namespace:    dv.Namespace,
		UID:          string(dv.UID),
		ClaimName:    dv.Status.ClaimName,
		Phase:        string(dv.Status.Phase),
		Progress:     string(dv.Status.Progress),
		RestartCount: dv.Status.RestartCount,
		Conditions:   conditions,
	}, nil
}

// GetPersistentVolumeClaim retrieves a PVC backing a CDI DataVolume.
func (p *KubeVirtProviderImpl) GetPersistentVolumeClaim(ctx context.Context, cluster, namespace, name string) (*domain.PersistentVolumeClaim, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	pvc, err := client.PVC().Get(opCtx, namespace, name, k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pvc %s/%s: %w", namespace, name, err)
	}

	return &domain.PersistentVolumeClaim{
		Name:                  pvc.Name,
		Namespace:             pvc.Namespace,
		Phase:                 string(pvc.Status.Phase),
		StorageClassName:      stringValue(pvc.Spec.StorageClassName),
		VolumeMode:            pvcVolumeMode(pvc.Spec.VolumeMode),
		RequestedStorageBytes: quantityValueBytes(pvc.Spec.Resources.Requests.Storage()),
		CapacityBytes:         quantityValueBytes(pvc.Status.Capacity.Storage()),
		CloneType:             strings.TrimSpace(pvc.Annotations["cdi.kubevirt.io/cloneType"]),
		ClonePhase:            strings.TrimSpace(pvc.Annotations["cdi.kubevirt.io/clonePhase"]),
		CloneFallbackReason:   strings.TrimSpace(pvc.Annotations["cdi.kubevirt.io/cloneFallbackReason"]),
	}, nil
}

// GetStorageClass retrieves a cluster-scoped StorageClass for clone-expansion preflight.
func (p *KubeVirtProviderImpl) GetStorageClass(ctx context.Context, cluster, name string) (*domain.StorageClass, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	storageClass, err := client.StorageClass().Get(opCtx, name, k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get storageclass %s: %w", name, err)
	}

	allowExpansion := false
	if storageClass.AllowVolumeExpansion != nil {
		allowExpansion = *storageClass.AllowVolumeExpansion
	}

	return &domain.StorageClass{
		Name:                 storageClass.Name,
		AllowVolumeExpansion: allowExpansion,
	}, nil
}

// GetStorageProfile retrieves the CDI StorageProfile for a target storage class.
func (p *KubeVirtProviderImpl) GetStorageProfile(ctx context.Context, cluster, name string) (*domain.StorageProfile, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	storageProfile, err := client.StorageProfile().Get(opCtx, name, k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get storageprofile %s: %w", name, err)
	}

	return mapStorageProfile(storageProfile), nil
}

// ListEventsForObject lists best-effort Kubernetes Events for the referenced object.
func (p *KubeVirtProviderImpl) ListEventsForObject(ctx context.Context, cluster string, ref domain.ObjectReference) ([]domain.ProvisioningEvent, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	if strings.TrimSpace(ref.Namespace) == "" {
		return nil, fmt.Errorf("event lookup requires namespace")
	}
	if strings.TrimSpace(ref.Kind) == "" || strings.TrimSpace(ref.Name) == "" {
		return nil, fmt.Errorf("event lookup requires kind and name")
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	selectors := []string{
		"involvedObject.kind=" + ref.Kind,
		"involvedObject.name=" + ref.Name,
	}
	if ref.UID != "" {
		selectors = append(selectors, "involvedObject.uid="+ref.UID)
	}

	list, err := client.Events().List(opCtx, ref.Namespace, k8smetav1.ListOptions{
		FieldSelector: strings.Join(selectors, ","),
	})
	if err != nil {
		return nil, fmt.Errorf("list events for %s/%s %s: %w", ref.Namespace, ref.Kind, ref.Name, err)
	}

	items := make([]domain.ProvisioningEvent, 0, len(list.Items))
	for i := range list.Items {
		ev := &list.Items[i]
		items = append(items, domain.ProvisioningEvent{
			Type:          ev.Type,
			Reason:        ev.Reason,
			Message:       ev.Message,
			Count:         ev.Count,
			FirstObserved: firstObservedTime(*ev),
			LastObserved:  lastObservedTime(*ev),
		})
	}
	return items, nil
}

func pvcVolumeMode(mode *corev1.PersistentVolumeMode) string {
	if mode == nil {
		return ""
	}
	return string(*mode)
}

func mapStorageProfile(storageProfile *cdiv1beta1.StorageProfile) *domain.StorageProfile {
	if storageProfile == nil {
		return &domain.StorageProfile{}
	}

	return &domain.StorageProfile{
		Name:              storageProfile.Name,
		CloneStrategy:     storageProfileCloneStrategy(storageProfile),
		DefaultVolumeMode: storageProfileDefaultVolumeMode(storageProfile),
		ClaimPropertySets: storageProfileClaimPropertySets(storageProfile),
	}
}

func storageProfileCloneStrategy(storageProfile *cdiv1beta1.StorageProfile) string {
	if storageProfile == nil {
		return ""
	}
	if storageProfile.Status.CloneStrategy != nil {
		return string(*storageProfile.Status.CloneStrategy)
	}
	if storageProfile.Spec.CloneStrategy != nil {
		return string(*storageProfile.Spec.CloneStrategy)
	}
	return ""
}

func storageProfileDefaultVolumeMode(storageProfile *cdiv1beta1.StorageProfile) string {
	if storageProfile == nil {
		return ""
	}

	claimPropertySets := effectiveStorageProfileClaimPropertySets(storageProfile)
	if len(claimPropertySets) == 0 || claimPropertySets[0].VolumeMode == nil {
		return ""
	}

	return string(*claimPropertySets[0].VolumeMode)
}

func storageProfileClaimPropertySets(storageProfile *cdiv1beta1.StorageProfile) []domain.StorageClaimPropertySet {
	if storageProfile == nil {
		return nil
	}

	rawSets := effectiveStorageProfileClaimPropertySets(storageProfile)
	if len(rawSets) == 0 {
		return nil
	}

	sets := make([]domain.StorageClaimPropertySet, 0, len(rawSets))
	for _, raw := range rawSets {
		set := domain.StorageClaimPropertySet{
			AccessModes: storageProfileAccessModes(raw.AccessModes),
		}
		if raw.VolumeMode != nil {
			set.VolumeMode = string(*raw.VolumeMode)
		}
		sets = append(sets, set)
	}
	return sets
}

func effectiveStorageProfileClaimPropertySets(storageProfile *cdiv1beta1.StorageProfile) []cdiv1beta1.ClaimPropertySet {
	if storageProfile == nil {
		return nil
	}

	claimPropertySets := storageProfile.Status.ClaimPropertySets
	if len(claimPropertySets) == 0 {
		claimPropertySets = storageProfile.Spec.ClaimPropertySets
	}
	return claimPropertySets
}

func storageProfileAccessModes(items []corev1.PersistentVolumeAccessMode) []string {
	if len(items) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := string(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// ListPodsUsingPVC returns non-terminal pods that currently reference the source PVC.
func (p *KubeVirtProviderImpl) ListPodsUsingPVC(
	ctx context.Context,
	cluster, namespace, claimName string,
) ([]domain.ObjectReference, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("pod lookup requires namespace")
	}
	if strings.TrimSpace(claimName) == "" {
		return nil, fmt.Errorf("pod lookup requires claim name")
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	list, err := client.Pods().List(opCtx, namespace, k8smetav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods in namespace %s: %w", namespace, err)
	}

	items := make([]domain.ObjectReference, 0)
	for i := range list.Items {
		pod := &list.Items[i]
		if isTerminalPodPhase(pod.Status.Phase) && pod.DeletionTimestamp == nil {
			continue
		}
		if !podUsesPVC(pod, claimName) {
			continue
		}
		items = append(items, domain.ObjectReference{
			Kind:      "Pod",
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       string(pod.UID),
		})
	}
	return items, nil
}

// CanClonePVCSource checks whether the current cluster credential can create the
// CDI clone source subresource in the source namespace.
func (p *KubeVirtProviderImpl) CanClonePVCSource(
	ctx context.Context,
	cluster, namespace string,
) (allowed bool, reason string, err error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return false, "", fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	if strings.TrimSpace(namespace) == "" {
		return false, "", fmt.Errorf("clone source access review requires namespace")
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	review, err := client.Authorization().CreateSelfSubjectAccessReview(opCtx, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      "create",
				Group:     "cdi.kubevirt.io",
				Resource:  "datavolumes/source",
			},
		},
	}, k8smetav1.CreateOptions{})
	if err != nil {
		return false, "", fmt.Errorf("create self subject access review for namespace %s: %w", namespace, err)
	}

	reason = strings.TrimSpace(review.Status.Reason)
	if evalErr := strings.TrimSpace(review.Status.EvaluationError); evalErr != "" {
		if reason != "" {
			reason += "; "
		}
		reason += evalErr
	}
	return review.Status.Allowed, reason, nil
}

// extractNameFromYAML extracts metadata.name from YAML bytes for safety validation.
// Used by UpdateVM to ensure the YAML target matches the function parameter.
func extractNameFromYAML(yamlData []byte) (string, error) {
	obj := &unstructured.Unstructured{}
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlData), 4096)
	if err := decoder.Decode(obj); err != nil {
		return "", fmt.Errorf("decode yaml for name extraction: %w", err)
	}
	name := obj.GetName()
	if name == "" {
		return "", fmt.Errorf("yaml does not contain metadata.name")
	}
	return name, nil
}

// validateYAMLResourceHalfSteps enforces CPU/Memory 0.5-step standards for any
// rendered YAML path, including caller-provided pre-rendered YAML.
func validateYAMLResourceHalfSteps(yamlData []byte) error {
	obj := &unstructured.Unstructured{}
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlData), 4096)
	if err := decoder.Decode(obj); err != nil {
		return fmt.Errorf("decode yaml: %w", err)
	}

	for path := range cpuResourcePaths {
		if err := validateNestedPathHalfStep(obj, path, validateCPUHalfStep); err != nil {
			return err
		}
	}
	for path := range memoryResourcePaths {
		if err := validateNestedPathHalfStep(obj, path, validateMemoryHalfStep); err != nil {
			return err
		}
	}
	return nil
}

func validateNestedPathHalfStep(
	obj *unstructured.Unstructured,
	path string,
	validateFn func(path string, value interface{}) error,
) error {
	value, found, err := unstructured.NestedFieldNoCopy(obj.Object, strings.Split(path, ".")...)
	if err != nil {
		return fmt.Errorf("read yaml field %q: %w", path, err)
	}
	if !found {
		return nil
	}
	return validateFn(path, value)
}

func firstObservedTime(ev corev1.Event) time.Time {
	if !ev.FirstTimestamp.IsZero() {
		return ev.FirstTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	return ev.CreationTimestamp.Time
}

func lastObservedTime(ev corev1.Event) time.Time {
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	return ev.CreationTimestamp.Time
}

func podUsesPVC(pod *corev1.Pod, claimName string) bool {
	if pod == nil {
		return false
	}
	want := strings.TrimSpace(claimName)
	if want == "" {
		return false
	}
	for i := range pod.Spec.Volumes {
		volume := &pod.Spec.Volumes[i]
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == want {
			return true
		}
	}
	return false
}

func isTerminalPodPhase(phase corev1.PodPhase) bool {
	return phase == corev1.PodSucceeded || phase == corev1.PodFailed
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func quantityValueBytes(q *resource.Quantity) int64 {
	if q == nil {
		return 0
	}
	return q.Value()
}
