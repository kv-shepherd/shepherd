package adminregistry

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"
)

// Registry stores available auth-provider admin adapters.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]admincontract.AuthProviderAdminAdapter
}

// New constructs a registry initialized with the provided adapters.
func New(initial ...admincontract.AuthProviderAdminAdapter) *Registry {
	r := &Registry{
		adapters: map[string]admincontract.AuthProviderAdminAdapter{},
	}
	for _, adapter := range initial {
		_ = r.Register(adapter)
	}
	return r
}

// Register registers an adapter by type. Duplicate type keys are rejected.
func (r *Registry) Register(adapter admincontract.AuthProviderAdminAdapter) error {
	if adapter == nil {
		return fmt.Errorf("adapter is nil")
	}
	t := strings.TrimSpace(strings.ToLower(adapter.Type()))
	if t == "" {
		return fmt.Errorf("adapter type is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[t]; exists {
		return fmt.Errorf("adapter type already registered: %s", t)
	}
	r.adapters[t] = adapter
	return nil
}

// Resolve returns a typed adapter when available, otherwise nil.
func (r *Registry) Resolve(authType string) admincontract.AuthProviderAdminAdapter {
	t := strings.TrimSpace(strings.ToLower(authType))
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.adapters[t]
}

// List returns all registered adapter descriptors sorted by type.
func (r *Registry) List() []admincontract.AuthProviderTypeDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]admincontract.AuthProviderTypeDescriptor, 0, len(r.adapters))
	for t, adapter := range r.adapters {
		if describer, ok := adapter.(admincontract.AuthProviderAdminAdapterDescriber); ok {
			desc := describer.Describe()
			desc.Type = strings.TrimSpace(strings.ToLower(desc.Type))
			if desc.Type == "" {
				desc.Type = t
			}
			if strings.TrimSpace(desc.DisplayName) == "" {
				desc.DisplayName = strings.ToUpper(desc.Type)
			}
			items = append(items, desc)
			continue
		}
		items = append(items, admincontract.AuthProviderTypeDescriptor{
			Type:        t,
			DisplayName: strings.ToUpper(t),
			BuiltIn:     false,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Type < items[j].Type })
	return items
}
