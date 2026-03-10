package handlers

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func provisioningToAPI(status *domain.ProvisioningStatus) *generated.ProvisioningStatus {
	if status == nil {
		return nil
	}

	conditions := make([]generated.ProvisioningCondition, 0, len(status.Conditions))
	for _, cond := range status.Conditions {
		conditions = append(conditions, generated.ProvisioningCondition{
			Type:               cond.Type,
			Status:             cond.Status,
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastTransitionTime: cond.LastTransitionTime,
		})
	}

	events := make([]generated.ProvisioningEvent, 0, len(status.RecentEvents))
	for _, ev := range status.RecentEvents {
		events = append(events, generated.ProvisioningEvent{
			Type:          ev.Type,
			Reason:        ev.Reason,
			Message:       ev.Message,
			Count:         int(ev.Count),
			FirstObserved: ev.FirstObserved,
			LastObserved:  ev.LastObserved,
		})
	}

	return &generated.ProvisioningStatus{
		RootDataVolumeName:  status.RootDataVolumeName,
		ClaimName:           status.ClaimName,
		Phase:               status.Phase,
		Progress:            status.Progress,
		RestartCount:        int(status.RestartCount),
		PvcPhase:            status.PvcPhase,
		CloneType:           status.CloneType,
		ClonePhase:          status.ClonePhase,
		CloneFallbackReason: status.CloneFallbackReason,
		FailureMessage:      status.FailureMessage,
		Conditions:          conditions,
		RecentEvents:        events,
	}
}

func (s *Server) loadVMProvisioning(ctx context.Context, vm *ent.VM) *generated.ProvisioningStatus {
	if s == nil || s.vmService == nil || vm == nil {
		return nil
	}
	if strings.TrimSpace(vm.ClusterID) == "" || strings.TrimSpace(vm.Namespace) == "" || strings.TrimSpace(vm.Name) == "" {
		return nil
	}

	status, err := s.vmService.GetProvisioningStatus(ctx, vm.ClusterID, vm.Namespace, vm.Name)
	if err != nil {
		logger.Warn("failed to load VM provisioning status",
			zap.Error(err),
			zap.String("vm_id", vm.ID),
			zap.String("cluster_id", vm.ClusterID),
			zap.String("namespace", vm.Namespace),
			zap.String("vm_name", vm.Name),
		)
		return nil
	}
	return provisioningToAPI(status)
}
