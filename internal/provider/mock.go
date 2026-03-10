package provider

import (
	"context"
	"fmt"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"kv-shepherd.io/shepherd/internal/domain"
)

// MockProvider implements InfrastructureProvider for testing without a K8s cluster.
type MockProvider struct {
	vms               map[string]*domain.VM                    // key: namespace/name
	dataVolumes       map[string]*domain.DataVolume            // key: namespace/name
	pvcs              map[string]*domain.PersistentVolumeClaim // key: namespace/name
	storageClasses    map[string]*domain.StorageClass          // key: name
	storageProfiles   map[string]*domain.StorageProfile        // key: name
	events            map[string][]domain.ProvisioningEvent    // key: namespace/kind/name
	pvcConsumers      map[string][]domain.ObjectReference      // key: namespace/claim
	cloneSourceAccess map[string]cloneSourceAccessDecision     // key: source namespace
	mu                sync.RWMutex
}

type cloneSourceAccessDecision struct {
	allowed bool
	reason  string
}

// NewMockProvider creates a new MockProvider.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		vms:               make(map[string]*domain.VM),
		dataVolumes:       make(map[string]*domain.DataVolume),
		pvcs:              make(map[string]*domain.PersistentVolumeClaim),
		storageClasses:    make(map[string]*domain.StorageClass),
		storageProfiles:   make(map[string]*domain.StorageProfile),
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
	p.dataVolumes = make(map[string]*domain.DataVolume)
	p.pvcs = make(map[string]*domain.PersistentVolumeClaim)
	p.storageClasses = make(map[string]*domain.StorageClass)
	p.storageProfiles = make(map[string]*domain.StorageProfile)
	p.events = make(map[string][]domain.ProvisioningEvent)
	p.pvcConsumers = make(map[string][]domain.ObjectReference)
	p.cloneSourceAccess = make(map[string]cloneSourceAccessDecision)
}

func (p *MockProvider) Name() string { return "mock" }
func (p *MockProvider) Type() string { return "mock" }

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
