package provider

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	kubevirtv1 "kubevirt.io/api/core/v1"

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

	return p.mapper.MapVM(vm, vmi)
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

	if vmList.Continue != "" {
		result.Continue = vmList.Continue
	}

	return result, nil
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

	if validateErr := validateYAMLResourceHalfSteps([]byte(spec.RenderedYAML)); validateErr != nil {
		return nil, fmt.Errorf("validate vm yaml resource steps for update: %w", validateErr)
	}

	// Safety check: validate YAML target name matches the `name` parameter.
	yamlName, err := extractNameFromYAML([]byte(spec.RenderedYAML))
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
	result, err := client.SSA().ApplyYAML(opCtx, namespace, []byte(spec.RenderedYAML))
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
