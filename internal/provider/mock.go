package provider

import (
	"context"
	"fmt"
	"sync"

	"kv-shepherd.io/shepherd/internal/domain"
)

// MockProvider implements InfrastructureProvider for testing without a K8s cluster.
type MockProvider struct {
	vms map[string]*domain.VM // key: namespace/name
	mu  sync.RWMutex
}

// NewMockProvider creates a new MockProvider.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		vms: make(map[string]*domain.VM),
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
