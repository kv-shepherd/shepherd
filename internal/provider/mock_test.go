package provider

import (
	"context"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/domain"
)

func TestMockProvider_DryRunVMMutation_RejectsInvalidMergePayload(t *testing.T) {
	t.Parallel()

	mock := NewMockProvider()
	err := mock.DryRunVMMutation(context.Background(), "cluster-a", "team-a", "vm-a", &domain.VMMutation{
		Mode:      domain.VMMutationModePatch,
		PatchType: domain.VMMutationPatchTypeMerge,
		Payload:   []byte(`{"spec":{"template":`),
	})
	if err == nil {
		t.Fatal("DryRunVMMutation() error = nil, want payload decode error")
	}
	if !strings.Contains(err.Error(), "decode vm merge mutation payload") {
		t.Fatalf("DryRunVMMutation() error = %v, want merge payload decode context", err)
	}
}

func TestMockProvider_DryRunVMMutation_RejectsUnsupportedPatchType(t *testing.T) {
	t.Parallel()

	mock := NewMockProvider()
	err := mock.DryRunVMMutation(context.Background(), "cluster-a", "team-a", "vm-a", &domain.VMMutation{
		Mode:      domain.VMMutationModePatch,
		PatchType: domain.VMMutationPatchTypeJSON,
		Payload:   []byte(`[{"op":"replace","path":"/spec/runStrategy","value":"Always"}]`),
	})
	if err == nil {
		t.Fatal("DryRunVMMutation() error = nil, want unsupported patch type error")
	}
	if !strings.Contains(err.Error(), "unsupported vm mutation patch type") {
		t.Fatalf("DryRunVMMutation() error = %v, want unsupported patch type context", err)
	}
}
