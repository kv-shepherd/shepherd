package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

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

func TestMockProvider_InstanceTypeCatalog(t *testing.T) {
	t.Parallel()

	mock := NewMockProvider()
	mock.SeedInstanceTypes("team-a", []*domain.InstanceType{
		{Name: "medium", CPU: 4, Memory: "8Gi"},
		{Name: "small", CPU: 2, Memory: "4Gi"},
	})
	mock.SeedInstanceTypes("team-b", []*domain.InstanceType{
		{Name: "other", CPU: 1, Memory: "2Gi"},
	})
	mock.SeedClusterInstanceTypes([]*domain.InstanceType{
		{Name: "cluster-large", CPU: 8, Memory: "16Gi"},
	})
	mock.SeedPreferences("team-a", []*domain.Preference{
		{Name: "linux"},
	})
	mock.SeedPreferences("team-b", []*domain.Preference{
		{Name: "windows"},
	})
	mock.SeedClusterPreferences([]*domain.Preference{
		{Name: "rhel"},
	})

	instanceTypes, err := mock.ListInstanceTypes(context.Background(), "cluster-a", "team-a")
	if err != nil {
		t.Fatalf("ListInstanceTypes() error = %v", err)
	}
	wantInstanceTypes := []*domain.InstanceType{
		{Name: "medium", CPU: 4, Memory: "8Gi"},
		{Name: "small", CPU: 2, Memory: "4Gi"},
	}
	if diff := cmp.Diff(wantInstanceTypes, instanceTypes); diff != "" {
		t.Fatalf("ListInstanceTypes() mismatch (-want +got):\n%s", diff)
	}

	clusterTypes, err := mock.ListClusterInstanceTypes(context.Background(), "cluster-a")
	if err != nil {
		t.Fatalf("ListClusterInstanceTypes() error = %v", err)
	}
	if diff := cmp.Diff([]*domain.InstanceType{{Name: "cluster-large", CPU: 8, Memory: "16Gi"}}, clusterTypes); diff != "" {
		t.Fatalf("ListClusterInstanceTypes() mismatch (-want +got):\n%s", diff)
	}

	preferences, err := mock.ListPreferences(context.Background(), "cluster-a", "team-a")
	if err != nil {
		t.Fatalf("ListPreferences() error = %v", err)
	}
	if diff := cmp.Diff([]*domain.Preference{{Name: "linux"}}, preferences); diff != "" {
		t.Fatalf("ListPreferences() mismatch (-want +got):\n%s", diff)
	}

	clusterPreferences, err := mock.ListClusterPreferences(context.Background(), "cluster-a")
	if err != nil {
		t.Fatalf("ListClusterPreferences() error = %v", err)
	}
	if diff := cmp.Diff([]*domain.Preference{{Name: "rhel"}}, clusterPreferences); diff != "" {
		t.Fatalf("ListClusterPreferences() mismatch (-want +got):\n%s", diff)
	}
}
