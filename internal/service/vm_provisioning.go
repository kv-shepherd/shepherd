package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
)

const maxProvisioningEvents = 5

// GetProvisioningStatus returns CDI/PVC-backed root-disk provisioning state for a VM.
// It is best-effort and returns nil when the provider does not expose provisioning
// queries or when the expected root DataVolume does not exist.
func (s *VMService) GetProvisioningStatus(
	ctx context.Context,
	cluster, namespace, vmName string,
) (*domain.ProvisioningStatus, error) {
	if s == nil || s.infra == nil {
		return nil, nil
	}
	observer, ok := s.infra.(provider.ProvisioningQueryProvider)
	if !ok {
		return nil, nil
	}

	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return nil, fmt.Errorf("vm name is required")
	}
	rootDVName := provider.DefaultRootDataVolumeName(vmName)

	dv, err := observer.GetDataVolume(ctx, cluster, namespace, rootDVName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get provisioning datavolume %s/%s: %w", namespace, rootDVName, err)
	}

	status := &domain.ProvisioningStatus{
		RootDataVolumeName: dv.Name,
		ClaimName:          dv.ClaimName,
		Phase:              dv.Phase,
		Progress:           dv.Progress,
		RestartCount:       dv.RestartCount,
		Conditions:         cloneProvisioningConditions(dv.Conditions),
	}

	if claimName := strings.TrimSpace(dv.ClaimName); claimName != "" {
		pvc, pvcErr := observer.GetPersistentVolumeClaim(ctx, cluster, namespace, claimName)
		switch {
		case pvcErr == nil:
			status.PvcPhase = pvc.Phase
			status.CloneType = pvc.CloneType
			status.ClonePhase = pvc.ClonePhase
			status.CloneFallbackReason = pvc.CloneFallbackReason
		case apierrors.IsNotFound(pvcErr):
			// Leave pvc_phase empty.
		}
	}

	events, eventsErr := observer.ListEventsForObject(ctx, cluster, domain.ObjectReference{
		Kind:      "DataVolume",
		Name:      dv.Name,
		Namespace: namespace,
		UID:       dv.UID,
	})
	if eventsErr == nil {
		status.RecentEvents = summarizeProvisioningEvents(events)
	}

	status.FailureMessage = deriveProvisioningFailureMessage(status)
	return status, nil
}

func cloneProvisioningConditions(src []domain.ProvisioningCondition) []domain.ProvisioningCondition {
	if len(src) == 0 {
		return nil
	}
	out := make([]domain.ProvisioningCondition, len(src))
	copy(out, src)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastTransitionTime.After(out[j].LastTransitionTime)
	})
	return out
}

func summarizeProvisioningEvents(src []domain.ProvisioningEvent) []domain.ProvisioningEvent {
	if len(src) == 0 {
		return nil
	}
	out := make([]domain.ProvisioningEvent, len(src))
	copy(out, src)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastObserved.After(out[j].LastObserved)
	})

	filtered := make([]domain.ProvisioningEvent, 0, len(out))
	for _, ev := range out {
		if strings.EqualFold(ev.Type, "Warning") {
			filtered = append(filtered, ev)
		}
	}
	if len(filtered) == 0 {
		filtered = out
	}
	if len(filtered) > maxProvisioningEvents {
		filtered = filtered[:maxProvisioningEvents]
	}
	return filtered
}

func deriveProvisioningFailureMessage(status *domain.ProvisioningStatus) string {
	if status == nil {
		return ""
	}
	for _, cond := range status.Conditions {
		if strings.EqualFold(cond.Status, "False") && strings.TrimSpace(cond.Message) != "" {
			return cond.Message
		}
	}
	if strings.EqualFold(status.Phase, "Failed") {
		for _, ev := range status.RecentEvents {
			if strings.EqualFold(ev.Type, "Warning") && strings.TrimSpace(ev.Message) != "" {
				return ev.Message
			}
		}
	}
	return ""
}
