package handlers

import "testing"

func float64Ptr(v float64) *float64 {
	return &v
}

func TestValidateInstanceSizeCreate_RejectsNonHalfStep(t *testing.T) {
	req := instanceSizeCreateRequest{
		Name:     "m4.large",
		CpuCores: 0.7,
		MemoryGi: 4,
	}
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected non-half-step cpu_cores to be rejected")
	}
}

func TestValidateInstanceSizeCreate_AcceptsHalfStep(t *testing.T) {
	req := instanceSizeCreateRequest{
		Name:            "m4.large",
		CpuCores:        1.5,
		MemoryGi:        3.5,
		CpuRequest:      float64Ptr(1.0),
		MemoryRequestGi: float64Ptr(2.5),
	}
	if err := validateInstanceSizeCreate(req); err != nil {
		t.Fatalf("expected valid half-step create request, got: %v", err)
	}
}

func TestValidateInstanceSizeUpdate_RejectsNonHalfStepRequest(t *testing.T) {
	req := instanceSizeUpdateRequest{
		CpuRequest: float64Ptr(0.7),
	}
	if err := validateInstanceSizeUpdate(req, false); err == nil {
		t.Fatal("expected non-half-step cpu_request to be rejected")
	}
}

func TestValidateInstanceSizeUpdate_AllowsZeroRequestClear(t *testing.T) {
	req := instanceSizeUpdateRequest{
		CpuRequest:      float64Ptr(0),
		MemoryRequestGi: float64Ptr(0),
	}
	if err := validateInstanceSizeUpdate(req, false); err != nil {
		t.Fatalf("expected zero clear sentinel to be accepted, got: %v", err)
	}
}
