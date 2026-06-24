package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"kv-shepherd.io/shepherd/internal/domain"
)

// MockProvider implements InfrastructureProvider for testing without a K8s cluster.
type MockProvider struct {
	vms               map[string]*domain.VM // key: namespace/name
	namespaces        map[string]struct{}
	dataVolumes       map[string]*domain.DataVolume            // key: namespace/name
	pvcs              map[string]*domain.PersistentVolumeClaim // key: namespace/name
	storageClasses    map[string]*domain.StorageClass          // key: name
	storageProfiles   map[string]*domain.StorageProfile        // key: name
	instanceTypes     map[string]*domain.InstanceType          // key: namespace/name
	clusterTypes      map[string]*domain.InstanceType          // key: name
	preferences       map[string]*domain.Preference            // key: namespace/name
	clusterPrefs      map[string]*domain.Preference            // key: name
	events            map[string][]domain.ProvisioningEvent    // key: namespace/kind/name
	pvcConsumers      map[string][]domain.ObjectReference      // key: namespace/claim
	cloneSourceAccess map[string]cloneSourceAccessDecision     // key: source namespace
	vncOpenErr        error
	serialOpenErr     error
	mu                sync.RWMutex
}

type cloneSourceAccessDecision struct {
	allowed bool
	reason  string
}

const mockProviderName = "mock"

// NewMockProvider creates a new MockProvider.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		vms:               make(map[string]*domain.VM),
		namespaces:        make(map[string]struct{}),
		dataVolumes:       make(map[string]*domain.DataVolume),
		pvcs:              make(map[string]*domain.PersistentVolumeClaim),
		storageClasses:    make(map[string]*domain.StorageClass),
		storageProfiles:   make(map[string]*domain.StorageProfile),
		instanceTypes:     make(map[string]*domain.InstanceType),
		clusterTypes:      make(map[string]*domain.InstanceType),
		preferences:       make(map[string]*domain.Preference),
		clusterPrefs:      make(map[string]*domain.Preference),
		events:            make(map[string][]domain.ProvisioningEvent),
		pvcConsumers:      make(map[string][]domain.ObjectReference),
		cloneSourceAccess: make(map[string]cloneSourceAccessDecision),
	}
}

// Seed populates the mock provider with test data.
func (p *MockProvider) Seed(vms []*domain.VM) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, vm := range vms {
		key := vm.Namespace + "/" + vm.Name
		p.vms[key] = vm
	}
}

// Reset clears all mock data.
func (p *MockProvider) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vms = make(map[string]*domain.VM)
	p.namespaces = make(map[string]struct{})
	p.dataVolumes = make(map[string]*domain.DataVolume)
	p.pvcs = make(map[string]*domain.PersistentVolumeClaim)
	p.storageClasses = make(map[string]*domain.StorageClass)
	p.storageProfiles = make(map[string]*domain.StorageProfile)
	p.instanceTypes = make(map[string]*domain.InstanceType)
	p.clusterTypes = make(map[string]*domain.InstanceType)
	p.preferences = make(map[string]*domain.Preference)
	p.clusterPrefs = make(map[string]*domain.Preference)
	p.events = make(map[string][]domain.ProvisioningEvent)
	p.pvcConsumers = make(map[string][]domain.ObjectReference)
	p.cloneSourceAccess = make(map[string]cloneSourceAccessDecision)
	p.vncOpenErr = nil
	p.serialOpenErr = nil
}

func (p *MockProvider) SetVNCOpenError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vncOpenErr = err
}

func (p *MockProvider) SetSerialOpenError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.serialOpenErr = err
}

func (p *MockProvider) Name() string { return mockProviderName }
func (p *MockProvider) Type() string { return mockProviderName }

func (p *MockProvider) EnsureNamespace(_ context.Context, _, namespace string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	p.namespaces[namespace] = struct{}{}
	return nil
}

func (p *MockProvider) GetVM(_ context.Context, _, namespace, name string) (*domain.VM, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	key := namespace + "/" + name
	vm, ok := p.vms[key]
	if !ok {
		return nil, fmt.Errorf("vm %s not found", key)
	}
	return vm, nil
}

func (p *MockProvider) GetVMManifestYAML(_ context.Context, _, namespace, name string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key := namespace + "/" + name
	vm, ok := p.vms[key]
	if !ok {
		return "", fmt.Errorf("vm %s not found", key)
	}

	manifest := map[string]interface{}{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]interface{}{
			"name":      vm.Name,
			"namespace": vm.Namespace,
		},
		"spec": map[string]interface{}{},
		"status": map[string]interface{}{
			"phase": string(vm.Status),
		},
	}

	jsonData, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal mock manifest json: %w", err)
	}
	yamlData, err := yaml.JSONToYAML(jsonData)
	if err != nil {
		return "", fmt.Errorf("convert mock manifest to yaml: %w", err)
	}
	return string(yamlData), nil
}

func (p *MockProvider) ListVMs(_ context.Context, _, namespace string, _ ListOptions) (*domain.VMList, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var items []*domain.VM
	for _, vm := range p.vms {
		if namespace == "" || vm.Namespace == namespace {
			items = append(items, vm)
		}
	}
	return &domain.VMList{Items: items, TotalCount: len(items)}, nil
}

func (p *MockProvider) CreateVM(_ context.Context, _, namespace string, spec *domain.VMSpec) (*domain.VM, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if spec == nil {
		spec = &domain.VMSpec{}
	}
	name := ""
	name = spec.Name
	if name == "" {
		name = fmt.Sprintf("mock-vm-%d", len(p.vms)+1)
	}
	vm := &domain.VM{
		Name:      name,
		Namespace: namespace,
		Status:    domain.VMStatusCreating,
		Spec:      *spec,
	}
	key := namespace + "/" + vm.Name
	p.vms[key] = vm
	return vm, nil
}

func (p *MockProvider) UpdateVM(_ context.Context, _, namespace, name string, spec *domain.VMSpec) (*domain.VM, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := namespace + "/" + name
	vm, ok := p.vms[key]
	if !ok {
		return nil, fmt.Errorf("vm %s not found", key)
	}
	vm.Spec = *spec
	return vm, nil
}

func (p *MockProvider) DryRunVMMutation(_ context.Context, _, namespace, name string, mutation *domain.VMMutation) error {
	if mutation == nil {
		return fmt.Errorf("vm mutation is nil")
	}
	if mutation.Mode != domain.VMMutationModePatch {
		return fmt.Errorf("unsupported vm mutation mode %q", mutation.Mode)
	}
	if namespace == "" || name == "" {
		return fmt.Errorf("vm mutation dry-run requires namespace and name")
	}
	if len(mutation.Payload) == 0 {
		return fmt.Errorf("vm mutation payload is empty")
	}
	switch mutation.PatchType {
	case "", domain.VMMutationPatchTypeMerge:
		if _, err := decodeMockMergeMutationPayload(mutation.Payload); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported vm mutation patch type %q", mutation.PatchType)
	}
	return nil
}

func (p *MockProvider) ExecuteVMMutation(ctx context.Context, cluster, namespace, name string, mutation *domain.VMMutation) (*domain.VM, error) {
	if err := p.DryRunVMMutation(ctx, cluster, namespace, name, mutation); err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	key := namespace + "/" + name
	vm, ok := p.vms[key]
	if !ok {
		return nil, fmt.Errorf("vm %s not found", key)
	}
	if len(mutation.Payload) == 0 {
		return vm, nil
	}

	patch, err := decodeMockMergeMutationPayload(mutation.Payload)
	if err != nil {
		return nil, err
	}

	specPatch, _ := patch["spec"].(map[string]interface{})
	if template, ok := specPatch["template"].(map[string]interface{}); ok {
		if templateSpec, ok := template["spec"].(map[string]interface{}); ok {
			if domainSpec, ok := templateSpec["domain"].(map[string]interface{}); ok {
				if cpu, ok := domainSpec["cpu"].(map[string]interface{}); ok {
					if value, ok := cpu["sockets"].(float64); ok {
						vm.Spec.CurrentCPUSockets = int(value)
					}
					if value, ok := cpu["cores"].(float64); ok {
						vm.Spec.CurrentCPUCoresPerSocket = int(value)
					}
					if value, ok := cpu["threads"].(float64); ok {
						vm.Spec.CurrentCPUThreads = int(value)
					}
				}
				if memory, ok := domainSpec["memory"].(map[string]interface{}); ok {
					if guest, ok := memory["guest"].(string); ok {
						var memoryGi float64
						if _, err := fmt.Sscanf(guest, "%fGi", &memoryGi); err == nil {
							vm.Spec.MemoryGi = memoryGi
						}
					}
				}
				if resources, ok := domainSpec["resources"].(map[string]interface{}); ok {
					if limits, ok := resources["limits"].(map[string]interface{}); ok {
						if cpu, ok := limits["cpu"].(string); ok {
							var value float64
							if _, err := fmt.Sscanf(cpu, "%f", &value); err == nil {
								vm.Spec.CPU = value
							}
						}
						if memory, ok := limits["memory"].(string); ok {
							var value float64
							if _, err := fmt.Sscanf(memory, "%fGi", &value); err == nil {
								vm.Spec.MemoryGi = value
							}
						}
					}
					if requests, ok := resources["requests"].(map[string]interface{}); ok {
						if cpu, ok := requests["cpu"].(string); ok {
							var value float64
							if _, err := fmt.Sscanf(cpu, "%f", &value); err == nil {
								vm.Spec.CPURequest = value
							}
						}
						if memory, ok := requests["memory"].(string); ok {
							var value float64
							if _, err := fmt.Sscanf(memory, "%fGi", &value); err == nil {
								vm.Spec.MemoryRequestGi = value
							}
						}
					}
				}
			}
		}
	}

	if dataVolumeTemplates, ok := specPatch["dataVolumeTemplates"].([]interface{}); ok && len(dataVolumeTemplates) > 0 {
		if item, ok := dataVolumeTemplates[0].(map[string]interface{}); ok {
			if spec, ok := item["spec"].(map[string]interface{}); ok {
				var resources map[string]interface{}
				switch {
				case spec["pvc"] != nil:
					if pvc, ok := spec["pvc"].(map[string]interface{}); ok {
						if value, ok := pvc["resources"].(map[string]interface{}); ok {
							resources = value
						}
					}
				case spec["storage"] != nil:
					if storage, ok := spec["storage"].(map[string]interface{}); ok {
						if value, ok := storage["resources"].(map[string]interface{}); ok {
							resources = value
						}
					}
				}
				if resources != nil {
					if requests, ok := resources["requests"].(map[string]interface{}); ok {
						if storage, ok := requests["storage"].(string); ok {
							var value int
							if _, err := fmt.Sscanf(storage, "%dGi", &value); err == nil {
								vm.Spec.DiskGB = value
							}
						}
					}
				}
			}
		}
	}

	return vm, nil
}

func decodeMockMergeMutationPayload(payload []byte) (map[string]interface{}, error) {
	var patch map[string]interface{}
	if err := json.Unmarshal(payload, &patch); err != nil {
		return nil, fmt.Errorf("decode vm merge mutation payload: %w", err)
	}
	return patch, nil
}

func (p *MockProvider) DeleteVM(_ context.Context, _, namespace, name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := namespace + "/" + name
	if _, ok := p.vms[key]; !ok {
		return fmt.Errorf("vm %s not found", key)
	}
	delete(p.vms, key)
	return nil
}

func (p *MockProvider) StartVM(_ context.Context, _, namespace, name string) error {
	return p.setStatus(namespace, name, domain.VMStatusRunning)
}

func (p *MockProvider) StopVM(_ context.Context, _, namespace, name string) error {
	return p.setStatus(namespace, name, domain.VMStatusStopped)
}

func (p *MockProvider) RestartVM(_ context.Context, _, namespace, name string) error {
	return p.setStatus(namespace, name, domain.VMStatusRunning)
}

func (p *MockProvider) OpenVNCStream(_ context.Context, _, namespace, name string) (net.Conn, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.vncOpenErr != nil {
		return nil, p.vncOpenErr
	}
	key := namespace + "/" + name
	if _, ok := p.vms[key]; !ok {
		return nil, fmt.Errorf("vm %s not found", key)
	}
	return mockNetConn{}, nil
}

func (p *MockProvider) OpenSerialConsoleStream(_ context.Context, _, namespace, name string) (net.Conn, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.serialOpenErr != nil {
		return nil, p.serialOpenErr
	}
	key := namespace + "/" + name
	if _, ok := p.vms[key]; !ok {
		return nil, fmt.Errorf("vm %s not found", key)
	}
	return mockNetConn{}, nil
}

type mockNetConn struct{}

func (mockNetConn) Read(_ []byte) (int, error)         { return 0, net.ErrClosed }
func (mockNetConn) Write(b []byte) (int, error)        { return len(b), nil }
func (mockNetConn) Close() error                       { return nil }
func (mockNetConn) LocalAddr() net.Addr                { return mockNetAddr("local") }
func (mockNetConn) RemoteAddr() net.Addr               { return mockNetAddr("remote") }
func (mockNetConn) SetDeadline(_ time.Time) error      { return nil }
func (mockNetConn) SetReadDeadline(_ time.Time) error  { return nil }
func (mockNetConn) SetWriteDeadline(_ time.Time) error { return nil }

type mockNetAddr string

func (a mockNetAddr) Network() string { return "mock" }
func (a mockNetAddr) String() string  { return string(a) }

func (p *MockProvider) PauseVM(_ context.Context, _, namespace, name string) error {
	return p.setStatus(namespace, name, domain.VMStatusStopped)
}

func (p *MockProvider) UnpauseVM(_ context.Context, _, namespace, name string) error {
	return p.setStatus(namespace, name, domain.VMStatusRunning)
}

func (p *MockProvider) ValidateSpec(_ context.Context, _, _ string, _ *domain.VMSpec) (*domain.ValidationResult, error) {
	return &domain.ValidationResult{Valid: true}, nil
}

func (p *MockProvider) SeedDataVolumes(items []*domain.DataVolume) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		if item == nil {
			continue
		}
		p.dataVolumes[item.Namespace+"/"+item.Name] = item
	}
}

func (p *MockProvider) SeedPVCs(items []*domain.PersistentVolumeClaim) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		if item == nil {
			continue
		}
		p.pvcs[item.Namespace+"/"+item.Name] = item
	}
}

func (p *MockProvider) SeedStorageClasses(items []*domain.StorageClass) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		if item == nil {
			continue
		}
		p.storageClasses[item.Name] = item
	}
}

func (p *MockProvider) SeedStorageProfiles(items []*domain.StorageProfile) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		if item == nil {
			continue
		}
		p.storageProfiles[item.Name] = item
	}
}

func (p *MockProvider) SeedInstanceTypes(namespace string, items []*domain.InstanceType) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		if item == nil {
			continue
		}
		p.instanceTypes[namespace+"/"+item.Name] = item
	}
}

func (p *MockProvider) SeedClusterInstanceTypes(items []*domain.InstanceType) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		if item == nil {
			continue
		}
		p.clusterTypes[item.Name] = item
	}
}

func (p *MockProvider) SeedPreferences(namespace string, items []*domain.Preference) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		if item == nil {
			continue
		}
		p.preferences[namespace+"/"+item.Name] = item
	}
}

func (p *MockProvider) SeedClusterPreferences(items []*domain.Preference) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		if item == nil {
			continue
		}
		p.clusterPrefs[item.Name] = item
	}
}

func (p *MockProvider) SeedEvents(ref domain.ObjectReference, items []domain.ProvisioningEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events[eventKey(ref)] = append([]domain.ProvisioningEvent(nil), items...)
}

func (p *MockProvider) SeedPVCConsumers(namespace, claimName string, items []domain.ObjectReference) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pvcConsumers[pvcConsumerKey(namespace, claimName)] = append([]domain.ObjectReference(nil), items...)
}

func (p *MockProvider) SetCloneSourceAccess(namespace string, allowed bool, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cloneSourceAccess[namespace] = cloneSourceAccessDecision{allowed: allowed, reason: reason}
}

func (p *MockProvider) GetDataVolume(_ context.Context, _, namespace, name string) (*domain.DataVolume, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	item, ok := p.dataVolumes[namespace+"/"+name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "cdi.kubevirt.io", Resource: "datavolumes"}, name)
	}
	return item, nil
}

func (p *MockProvider) GetPersistentVolumeClaim(_ context.Context, _, namespace, name string) (*domain.PersistentVolumeClaim, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	item, ok := p.pvcs[namespace+"/"+name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "persistentvolumeclaims"}, name)
	}
	return item, nil
}

func (p *MockProvider) GetStorageClass(_ context.Context, _, name string) (*domain.StorageClass, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	item, ok := p.storageClasses[name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "storage.k8s.io", Resource: "storageclasses"}, name)
	}
	return item, nil
}

func (p *MockProvider) GetStorageProfile(_ context.Context, _, name string) (*domain.StorageProfile, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	item, ok := p.storageProfiles[name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "cdi.kubevirt.io", Resource: "storageprofiles"}, name)
	}
	return item, nil
}

func (p *MockProvider) ListInstanceTypes(_ context.Context, _, namespace string) ([]*domain.InstanceType, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return sortedInstanceTypeValues(p.instanceTypes, namespace+"/"), nil
}

func (p *MockProvider) ListClusterInstanceTypes(_ context.Context, _ string) ([]*domain.InstanceType, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return sortedInstanceTypeValues(p.clusterTypes, ""), nil
}

func (p *MockProvider) ListPreferences(_ context.Context, _, namespace string) ([]*domain.Preference, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return sortedPreferenceValues(p.preferences, namespace+"/"), nil
}

func (p *MockProvider) ListClusterPreferences(_ context.Context, _ string) ([]*domain.Preference, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return sortedPreferenceValues(p.clusterPrefs, ""), nil
}

func (p *MockProvider) ListEventsForObject(_ context.Context, _ string, ref domain.ObjectReference) ([]domain.ProvisioningEvent, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	items := p.events[eventKey(ref)]
	out := make([]domain.ProvisioningEvent, len(items))
	copy(out, items)
	return out, nil
}

func (p *MockProvider) ListPodsUsingPVC(_ context.Context, _, namespace, claimName string) ([]domain.ObjectReference, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	items := p.pvcConsumers[pvcConsumerKey(namespace, claimName)]
	out := make([]domain.ObjectReference, len(items))
	copy(out, items)
	return out, nil
}

func (p *MockProvider) CanClonePVCSource(_ context.Context, _, namespace string) (allowed bool, reason string, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	decision, ok := p.cloneSourceAccess[namespace]
	if !ok {
		return true, "", nil
	}
	return decision.allowed, decision.reason, nil
}

func (p *MockProvider) setStatus(namespace, name string, status domain.VMStatus) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := namespace + "/" + name
	vm, ok := p.vms[key]
	if !ok {
		return fmt.Errorf("vm %s not found", key)
	}
	vm.Status = status
	return nil
}

func eventKey(ref domain.ObjectReference) string {
	return ref.Namespace + "/" + ref.Kind + "/" + ref.Name
}

func pvcConsumerKey(namespace, claimName string) string {
	return namespace + "/" + claimName
}

func sortedInstanceTypeValues(items map[string]*domain.InstanceType, prefix string) []*domain.InstanceType {
	keys := make([]string, 0, len(items))
	for key := range items {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]*domain.InstanceType, 0, len(keys))
	for _, key := range keys {
		out = append(out, items[key])
	}
	return out
}

func sortedPreferenceValues(items map[string]*domain.Preference, prefix string) []*domain.Preference {
	keys := make([]string, 0, len(items))
	for key := range items {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]*domain.Preference, 0, len(keys))
	for _, key := range keys {
		out = append(out, items[key])
	}
	return out
}
