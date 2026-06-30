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

// InstanceSizeUsesHugepages reports whether an instance size requires hugepages,
// either through indexed capability fields or spec overrides.
func InstanceSizeUsesHugepages(size *ent.InstanceSize) bool {
	if size == nil {
		return false
	}
	hugepagesSize := normalizeHugepagesSize(size.HugepagesSize)
	if hugepagesSize == "" {
		hugepagesSize = normalizeHugepagesSize(extractHugepagesSize(size.SpecOverrides))
	}
	return size.RequiresHugepages || hugepagesSize != ""
}

// ValidateHugepagesMemoryRequestGi enforces the Kubernetes hugepages scheduling
// rule: hugepages-backed memory cannot be overcommitted, so memory request must
// be explicitly set and equal the effective memory limit.
func ValidateHugepagesMemoryRequestGi(
	size *ent.InstanceSize,
	memoryLimitGi float64,
	memoryRequestGi float64,
) error {
	if !InstanceSizeUsesHugepages(size) {
		return nil
	}
	if memoryLimitGi <= 0 {
		return fmt.Errorf("hugepages-backed memory requires memory limit to be > 0")
	}
	if memoryRequestGi <= 0 {
		return fmt.Errorf("hugepages-backed memory requires explicit memory_request_gi equal to memory_gi")
	}
	if memoryRequestGi != memoryLimitGi {
		return fmt.Errorf(
			"hugepages-backed memory requires memory_request_gi (%.1fGi) to equal memory_gi (%.1fGi)",
			memoryRequestGi,
			memoryLimitGi,
		)
	}
	return nil
}

// AlignHugepagesMemoryRequestGi returns the aligned memory request only after
// the base hugepages request is explicitly configured. It never converts a zero
// request into a limit; zero is a configuration error.
func AlignHugepagesMemoryRequestGi(
	size *ent.InstanceSize,
	memoryLimitGi float64,
	memoryRequestGi float64,
) (alignedRequestGi float64, adjusted bool, err error) {
	if !InstanceSizeUsesHugepages(size) {
		return memoryRequestGi, false, nil
	}
	if memoryRequestGi <= 0 {
		return 0, false, fmt.Errorf("hugepages-backed memory requires explicit memory_request_gi equal to memory_gi")
	}
	if memoryLimitGi <= 0 {
		return 0, false, fmt.Errorf("hugepages-backed memory requires memory limit to be > 0")
	}
	if memoryRequestGi == memoryLimitGi {
		return memoryRequestGi, false, nil
	}
	return memoryLimitGi, true, nil
}
