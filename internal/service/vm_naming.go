package service

import (
	"context"
	"fmt"

	"kv-shepherd.io/shepherd/ent"
)

// VMNamingService generates platform-managed VM names per ADR-0017/master-flow Stage 5.C.
// Pattern: {namespace}-{system_name}-{service_name}-{instance_index}
// Example: prod-shop-redis-01
type VMNamingService struct {
	client *ent.Client
}

// NewVMNamingService creates a new VMNamingService.
func NewVMNamingService(client *ent.Client) *VMNamingService {
	return &VMNamingService{client: client}
}

// GenerateVMName generates a unique VM name and increments the service's next_instance_index.
// This MUST be called within a transaction to ensure atomicity.
func (s *VMNamingService) GenerateVMName(ctx context.Context, namespace string, serviceID string) (name string, instance string, err error) {
	svcEnt, err := s.client.Service.Get(ctx, serviceID)
	if err != nil {
		return "", "", fmt.Errorf("service not found: %s: %w", serviceID, err)
	}

	sysEnt, err := svcEnt.QuerySystem().Only(ctx)
	if err != nil {
		return "", "", fmt.Errorf("system not found for service %s: %w", serviceID, err)
	}

	// Get current index and increment (permanently incrementing, no reset per ADR-0015 §2).
	idx := svcEnt.NextInstanceIndex

	// Format instance as zero-padded 2-digit string.
	instance = fmt.Sprintf("%02d", idx)

	// Generate name: {namespace}-{system}-{service}-{idx}
	name = fmt.Sprintf("%s-%s-%s-%s", namespace, sysEnt.Name, svcEnt.Name, instance)

	// Increment next_instance_index atomically.
	err = s.client.Service.UpdateOneID(serviceID).
		SetNextInstanceIndex(idx + 1).
		Exec(ctx)
	if err != nil {
		return "", "", fmt.Errorf("increment instance index: %w", err)
	}

	return name, instance, nil
}
