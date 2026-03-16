package service

import (
	"testing"
	"time"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
)

func TestGetProvisioningStatus_ReturnsNil_WhenProviderUnavailable(t *testing.T) {
	t.Parallel()

	svc := NewVMService(nil)
	got, err := svc.GetProvisioningStatus(t.Context(), "cluster-a", "team-a", "vm-a")
	if err != nil {
		t.Fatalf("GetProvisioningStatus error = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("GetProvisioningStatus = %#v, want nil", got)
	}
}

func TestGetProvisioningStatus_ReturnsNil_WhenRootDataVolumeMissing(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	svc := NewVMService(mock)

	got, err := svc.GetProvisioningStatus(t.Context(), "cluster-a", "team-a", "vm-a")
	if err != nil {
		t.Fatalf("GetProvisioningStatus error = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("GetProvisioningStatus = %#v, want nil", got)
	}
}

func TestGetProvisioningStatus_AggregatesDataVolumePVCAndEvents(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	mock := provider.NewMockProvider()
	mock.SeedDataVolumes([]*domain.DataVolume{
		{
			Name:         "vm-a-rootfs",
			Namespace:    "team-a",
			UID:          "dv-uid-1",
			ClaimName:    "vm-a-rootfs",
			Phase:        "Failed",
			Progress:     "75.0%",
			RestartCount: 2,
			Conditions: []domain.ProvisioningCondition{
				{
					Type:               "Ready",
					Status:             "False",
					Reason:             "CloneFailed",
					Message:            "smart clone permission denied",
					LastTransitionTime: now.Add(-1 * time.Minute),
				},
				{
					Type:               "Bound",
					Status:             "True",
					Reason:             "PVCBound",
					LastTransitionTime: now.Add(-5 * time.Minute),
				},
			},
		},
	})
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{
		{
			Name:                "vm-a-rootfs",
			Namespace:           "team-a",
			Phase:               "Bound",
			CloneType:           "copy",
			ClonePhase:          "Succeeded",
			CloneFallbackReason: "The volume modes of source and target are incompatible",
		},
	})
	mock.SeedEvents(domain.ObjectReference{
		Kind:      "DataVolume",
		Name:      "vm-a-rootfs",
		Namespace: "team-a",
		UID:       "dv-uid-1",
	}, []domain.ProvisioningEvent{
		{
			Type:          "Normal",
			Reason:        "CloneScheduled",
			Message:       "clone scheduled",
			LastObserved:  now.Add(-10 * time.Minute),
			FirstObserved: now.Add(-10 * time.Minute),
		},
		{
			Type:          "Warning",
			Reason:        "CloneSourceDenied",
			Message:       "source PVC clone RBAC missing",
			LastObserved:  now.Add(-30 * time.Second),
			FirstObserved: now.Add(-45 * time.Second),
			Count:         3,
		},
	})

	svc := NewVMService(mock)
	got, err := svc.GetProvisioningStatus(t.Context(), "cluster-a", "team-a", "vm-a")
	if err != nil {
		t.Fatalf("GetProvisioningStatus error = %v, want nil", err)
	}
	if got == nil {
		t.Fatal("GetProvisioningStatus = nil, want aggregated status")
	}
	if got.RootDataVolumeName != "vm-a-rootfs" {
		t.Fatalf("RootDataVolumeName = %q, want %q", got.RootDataVolumeName, "vm-a-rootfs")
	}
	if got.ClaimName != "vm-a-rootfs" {
		t.Fatalf("ClaimName = %q, want %q", got.ClaimName, "vm-a-rootfs")
	}
	if got.Phase != "Failed" {
		t.Fatalf("Phase = %q, want %q", got.Phase, "Failed")
	}
	if got.Progress != "75.0%" {
		t.Fatalf("Progress = %q, want %q", got.Progress, "75.0%")
	}
	if got.RestartCount != 2 {
		t.Fatalf("RestartCount = %d, want 2", got.RestartCount)
	}
	if got.PvcPhase != "Bound" {
		t.Fatalf("PvcPhase = %q, want %q", got.PvcPhase, "Bound")
	}
	if got.CloneType != "copy" {
		t.Fatalf("CloneType = %q, want %q", got.CloneType, "copy")
	}
	if got.ClonePhase != "Succeeded" {
		t.Fatalf("ClonePhase = %q, want %q", got.ClonePhase, "Succeeded")
	}
	if got.CloneFallbackReason != "The volume modes of source and target are incompatible" {
		t.Fatalf("CloneFallbackReason = %q, want expected fallback reason", got.CloneFallbackReason)
	}
	if got.FailureMessage != "smart clone permission denied" {
		t.Fatalf("FailureMessage = %q, want %q", got.FailureMessage, "smart clone permission denied")
	}
	if len(got.Conditions) != 2 {
		t.Fatalf("len(Conditions) = %d, want 2", len(got.Conditions))
	}
	if got.Conditions[0].Type != "Ready" {
		t.Fatalf("Conditions[0].Type = %q, want %q", got.Conditions[0].Type, "Ready")
	}
	if len(got.RecentEvents) != 1 {
		t.Fatalf("len(RecentEvents) = %d, want 1 warning event", len(got.RecentEvents))
	}
	if got.RecentEvents[0].Reason != "CloneSourceDenied" {
		t.Fatalf("RecentEvents[0].Reason = %q, want %q", got.RecentEvents[0].Reason, "CloneSourceDenied")
	}
}
