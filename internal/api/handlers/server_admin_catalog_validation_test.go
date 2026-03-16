package handlers

import (
	"testing"

	"kv-shepherd.io/shepherd/ent"
)

func float64Ptr(v float64) *float64 {
	return &v
}

func TestValidateInstanceSizeCreate_RejectsNonHalfStep(t *testing.T) {
	req := instanceSizeCreateRequest{
		Name:     "m4.large",
		CPUCores: 0.7,
		MemoryGi: 4,
	}
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected non-half-step cpu_cores to be rejected")
	}
}

func TestValidateInstanceSizeCreate_AcceptsHalfStep(t *testing.T) {
	req := instanceSizeCreateRequest{
		Name:            "m4.large",
		CPUCores:        1.5,
		MemoryGi:        3.5,
		CPURequest:      float64Ptr(1.0),
		MemoryRequestGi: float64Ptr(2.5),
	}
	if err := validateInstanceSizeCreate(req); err != nil {
		t.Fatalf("expected valid half-step create request, got: %v", err)
	}
}

func TestValidateInstanceSizeUpdate_RejectsNonHalfStepRequest(t *testing.T) {
	req := instanceSizeUpdateRequest{
		CPURequest: float64Ptr(0.7),
	}
	if err := validateInstanceSizeUpdate(req, &ent.InstanceSize{CPUCores: 2, MemoryGi: 4}); err == nil {
		t.Fatal("expected non-half-step cpu_request to be rejected")
	}
}

func TestValidateInstanceSizeUpdate_AllowsZeroRequestClear(t *testing.T) {
	req := instanceSizeUpdateRequest{
		CPURequest:      float64Ptr(0),
		MemoryRequestGi: float64Ptr(0),
	}
	if err := validateInstanceSizeUpdate(req, &ent.InstanceSize{
		CPUCores:        2,
		CPURequest:      1,
		MemoryGi:        4,
		MemoryRequestGi: 2,
	}); err != nil {
		t.Fatalf("expected zero clear sentinel to be accepted, got: %v", err)
	}
}

func TestValidateInstanceSizeCreate_RejectsRequestGreaterThanLimit(t *testing.T) {
	req := instanceSizeCreateRequest{
		Name:            "invalid-overcommit",
		CPUCores:        2,
		CPURequest:      float64Ptr(4),
		MemoryGi:        4,
		MemoryRequestGi: float64Ptr(2),
	}
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected cpu_request > cpu_cores to be rejected")
	}
}

func TestValidateInstanceSizeUpdate_RejectsEffectiveRequestGreaterThanLimit(t *testing.T) {
	req := instanceSizeUpdateRequest{
		CPUCores: float64Ptr(2),
	}
	if err := validateInstanceSizeUpdate(req, &ent.InstanceSize{
		CPUCores:   4,
		CPURequest: 4,
		MemoryGi:   8,
	}); err == nil {
		t.Fatal("expected lowering cpu_cores below existing cpu_request to be rejected")
	}
}
