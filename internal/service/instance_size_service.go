package service

import (
	"context"
	"fmt"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/instancesize"
)

// InstanceSizeService handles instance size business logic (ADR-0018).
type InstanceSizeService struct {
	client *ent.Client
}

// NewInstanceSizeService creates a new InstanceSizeService.
func NewInstanceSizeService(client *ent.Client) *InstanceSizeService {
	return &InstanceSizeService{client: client}
}

// ListEnabled returns all enabled instance sizes ordered by sort_order.
func (s *InstanceSizeService) ListEnabled(ctx context.Context) ([]*ent.InstanceSize, error) {
	sizes, err := s.client.InstanceSize.Query().
		Where(instancesize.EnabledEQ(true)).
		Order(ent.Asc(instancesize.FieldSortOrder)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list instance sizes: %w", err)
	}
	return sizes, nil
}

// GetByID returns an instance size by ID.
func (s *InstanceSizeService) GetByID(ctx context.Context, id string) (*ent.InstanceSize, error) {
	size, err := s.client.InstanceSize.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get instance size %s: %w", id, err)
	}
	return size, nil
}

// GetEffectiveCPURequest returns cpu_request if set, otherwise cpu_cores (no overcommit default).
func GetEffectiveCPURequest(size *ent.InstanceSize) int {
	if size.CPURequest > 0 {
		return size.CPURequest
	}
	return size.CPUCores
}

// GetEffectiveMemoryRequest returns memory_request_mb if set, otherwise memory_mb.
func GetEffectiveMemoryRequest(size *ent.InstanceSize) int {
	if size.MemoryRequestMB > 0 {
		return size.MemoryRequestMB
	}
	return size.MemoryMB
}
