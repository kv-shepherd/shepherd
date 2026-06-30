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

func TestValidateInstanceSizeCreate_DedicatedCPURequiresExplicitRequest(t *testing.T) {
	dedicated := true
	req := instanceSizeCreateRequest{
		Name:            "m4.dedicated",
		CPUCores:        4,
		MemoryGi:        8,
		MemoryRequestGi: float64Ptr(8),
		DedicatedCPU:    &dedicated,
	}
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected dedicated cpu without cpu_request to be rejected")
	}

	zero := 0.0
	req.CPURequest = &zero
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected dedicated cpu with cpu_request=0 to be rejected")
	}

	req.CPURequest = float64Ptr(4)
	if err := validateInstanceSizeCreate(req); err != nil {
		t.Fatalf("expected dedicated cpu with request=limit to pass, got: %v", err)
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

func TestValidateInstanceSizeUpdate_DedicatedCPURejectsClearingRequest(t *testing.T) {
	req := instanceSizeUpdateRequest{
		CPURequest: float64Ptr(0),
	}
	if err := validateInstanceSizeUpdate(req, &ent.InstanceSize{
		CPUCores:     4,
		CPURequest:   4,
		MemoryGi:     8,
		DedicatedCPU: true,
	}); err == nil {
		t.Fatal("expected clearing cpu_request on dedicated cpu to be rejected")
	}
}

func TestValidateInstanceSizeUpdate_RejectsZeroRequestClear(t *testing.T) {
	req := instanceSizeUpdateRequest{
		CPURequest:      float64Ptr(0),
		MemoryRequestGi: float64Ptr(0),
	}
	if err := validateInstanceSizeUpdate(req, &ent.InstanceSize{
		CPUCores:        2,
		CPURequest:      1,
		MemoryGi:        4,
		MemoryRequestGi: 2,
	}); err == nil {
		t.Fatal("expected zero request clear sentinel to be rejected")
	}
}

func TestValidateInstanceSizeCreate_RejectsMissingRequests(t *testing.T) {
	req := instanceSizeCreateRequest{
		Name:     "m4.shared",
		CPUCores: 4,
		MemoryGi: 8,
	}
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected missing cpu_request and memory_request_gi to be rejected")
	}

	req.CPURequest = float64Ptr(2)
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected missing memory_request_gi to be rejected")
	}
}

func TestValidateInstanceSizeCreate_HugepagesRequiresAlignedMemoryRequest(t *testing.T) {
	requiresHugepages := true
	hugepagesSize := "2Mi"
	req := instanceSizeCreateRequest{
		Name:              "m4.hugepages",
		CPUCores:          4,
		CPURequest:        float64Ptr(4),
		MemoryGi:          8,
		RequiresHugepages: &requiresHugepages,
		HugepagesSize:     &hugepagesSize,
	}
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected hugepages without memory_request_gi to be rejected")
	}

	req.MemoryRequestGi = float64Ptr(0)
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected hugepages with memory_request_gi=0 to be rejected")
	}

	req.MemoryRequestGi = float64Ptr(4)
	if err := validateInstanceSizeCreate(req); err == nil {
		t.Fatal("expected hugepages with memory_request_gi below memory_gi to be rejected")
	}

	req.MemoryRequestGi = float64Ptr(8)
	if err := validateInstanceSizeCreate(req); err != nil {
		t.Fatalf("expected hugepages with memory_request_gi=memory_gi to pass, got: %v", err)
	}
}

func TestValidateInstanceSizeUpdate_HugepagesRejectsClearingMemoryRequest(t *testing.T) {
	req := instanceSizeUpdateRequest{
		MemoryRequestGi: float64Ptr(0),
	}
	if err := validateInstanceSizeUpdate(req, &ent.InstanceSize{
		CPUCores:          4,
		CPURequest:        4,
		MemoryGi:          8,
		MemoryRequestGi:   8,
		RequiresHugepages: true,
		HugepagesSize:     "2Mi",
	}); err == nil {
		t.Fatal("expected clearing memory_request_gi on hugepages size to be rejected")
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
		CPUCores:        4,
		CPURequest:      4,
		MemoryGi:        8,
		MemoryRequestGi: 8,
	}); err == nil {
		t.Fatal("expected lowering cpu_cores below existing cpu_request to be rejected")
	}
}
