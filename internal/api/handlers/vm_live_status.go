package handlers

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/ent"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	infracontract "kv-shepherd.io/shepherd/internal/provider/infracontract"
)

const liveVMStatusPersistMinInterval = 15 * time.Second
const liveVMCreateBootstrapGraceWindow = 2 * time.Minute

type vmLiveGroupKey struct {
	clusterID string
	namespace string
}

func (s *Server) refreshVMLiveState(ctx context.Context, vmRow *ent.VM) *ent.VM {
	if s == nil || s.vmService == nil || vmRow == nil || strings.TrimSpace(vmRow.ClusterID) == "" {
		return vmRow
	}

	liveVM, err := s.vmService.GetVM(ctx, vmRow.ClusterID, vmRow.Namespace, vmRow.Name)
	if err != nil {
		// Distinguish K8s NotFound (resource gone) from other errors (cluster unreachable).
		isNotFound := k8serrors.IsNotFound(err)
		logger.Warn("failed to refresh live vm status",
			zap.String("vm_id", vmRow.ID),
			zap.String("cluster_id", vmRow.ClusterID),
			zap.String("namespace", vmRow.Namespace),
			zap.String("name", vmRow.Name),
			zap.Bool("is_not_found", isNotFound),
			zap.Error(err),
		)
		return s.applyUnavailableVMState(ctx, vmRow, time.Now(), isNotFound)
	}

	return s.applyObservedVMState(ctx, vmRow, liveVM, time.Now())
}

func (s *Server) refreshVMLiveStates(ctx context.Context, vms []*ent.VM) []*ent.VM {
	if s == nil || s.vmService == nil || len(vms) == 0 {
		return vms
	}

	groups := make(map[vmLiveGroupKey][]int)
	for i, vmRow := range vms {
		if vmRow == nil || strings.TrimSpace(vmRow.ClusterID) == "" || strings.TrimSpace(vmRow.Namespace) == "" {
			continue
		}
		key := vmLiveGroupKey{
			clusterID: vmRow.ClusterID,
			namespace: vmRow.Namespace,
		}
		groups[key] = append(groups[key], i)
	}

	for key, indexes := range groups {
		groupRows := make([]*ent.VM, 0, len(indexes))
		for _, idx := range indexes {
			if vms[idx] != nil {
				groupRows = append(groupRows, vms[idx])
			}
		}

		liveList, err := s.listVMLiveStateGroup(ctx, key, groupRows)
		observedAt := time.Now()
		if err != nil {
			// Scenario A: K8s API call failed — cluster unreachable → UNKNOWN
			logger.Warn("failed to refresh live vm statuses for namespace",
				zap.String("cluster_id", key.clusterID),
				zap.String("namespace", key.namespace),
				zap.Error(err),
			)
			for _, idx := range indexes {
				if vms[idx] == nil {
					continue
				}
				vms[idx] = s.applyUnavailableVMState(ctx, vms[idx], observedAt, false)
			}
			continue
		}

		// Scenario B: K8s API returned successfully — build lookup map.
		liveByKey := make(map[string]*domain.VM)
		if liveList != nil {
			for _, item := range liveList.Items {
				if item == nil {
					continue
				}
				liveByKey[item.Namespace+"/"+item.Name] = item
			}
		}

		for _, idx := range indexes {
			vmRow := vms[idx]
			if vmRow == nil {
				continue
			}
			liveVM := liveByKey[vmRow.Namespace+"/"+vmRow.Name]
			if liveVM == nil {
				// Scenario B2: cluster responded OK but VM not found → NOT_FOUND
				vms[idx] = s.applyUnavailableVMState(ctx, vmRow, observedAt, true)
				continue
			}
			vms[idx] = s.applyObservedVMState(ctx, vmRow, liveVM, observedAt)
		}
	}

	return vms
}

func (s *Server) loadObservedLiveVMsByID(ctx context.Context, vms []*ent.VM) map[string]*domain.VM {
	liveByVMID := make(map[string]*domain.VM, len(vms))
	if s == nil || s.vmService == nil || len(vms) == 0 {
		return liveByVMID
	}

	groups := make(map[vmLiveGroupKey][]*ent.VM)
	for _, vmRow := range vms {
		if vmRow == nil || strings.TrimSpace(vmRow.ClusterID) == "" || strings.TrimSpace(vmRow.Namespace) == "" {
			continue
		}
		key := vmLiveGroupKey{
			clusterID: vmRow.ClusterID,
			namespace: vmRow.Namespace,
		}
		groups[key] = append(groups[key], vmRow)
	}

	for key, group := range groups {
		liveList, err := s.listVMLiveStateGroup(ctx, key, group)
		if err != nil {
			logger.Warn("failed to load live vm details for namespace",
				zap.String("cluster_id", key.clusterID),
				zap.String("namespace", key.namespace),
				zap.Error(err),
			)
			continue
		}

		liveByName := make(map[string]*domain.VM)
		if liveList != nil {
			for _, item := range liveList.Items {
				if item == nil {
					continue
				}
				liveByName[item.Namespace+"/"+item.Name] = item
			}
		}

		for _, vmRow := range group {
			if vmRow == nil {
				continue
			}
			if liveVM := liveByName[vmRow.Namespace+"/"+vmRow.Name]; liveVM != nil {
				liveByVMID[vmRow.ID] = liveVM
			}
		}
	}

	return liveByVMID
}

func (s *Server) listVMLiveStateGroup(ctx context.Context, key vmLiveGroupKey, group []*ent.VM) (*domain.VMList, error) {
	resourceVersion := liveStateGroupResourceVersion(group)
	liveList, err := s.vmService.ListVMs(ctx, key.clusterID, key.namespace, infracontract.ListOptions{
		ResourceVersion: resourceVersion,
	})
	if err == nil || resourceVersion == "" {
		return liveList, err
	}
	if !k8serrors.IsResourceExpired(err) && !k8serrors.IsGone(err) {
		return liveList, err
	}

	logger.Warn("cached resourceVersion expired while refreshing live vm statuses, retrying baseline",
		zap.String("cluster_id", key.clusterID),
		zap.String("namespace", key.namespace),
		zap.String("stale_rv", resourceVersion),
		zap.Error(err),
	)
	return s.vmService.ListVMs(ctx, key.clusterID, key.namespace, infracontract.ListOptions{
		ResourceVersion: "",
	})
}

func liveStateGroupResourceVersion(group []*ent.VM) string {
	for _, vmRow := range group {
		if vmRow == nil || vmRow.LastK8sRv == nil {
			continue
		}
		if resourceVersion := strings.TrimSpace(*vmRow.LastK8sRv); resourceVersion != "" {
			return resourceVersion
		}
	}
	return ""
}

func (s *Server) applyObservedVMState(ctx context.Context, vmRow *ent.VM, liveVM *domain.VM, observedAt time.Time) *ent.VM {
	if s == nil || s.client == nil || vmRow == nil || liveVM == nil {
		return vmRow
	}

	newStatus := mapDomainVMStatusToEntVM(liveVM.Status)
	newTier := pollingTierForVMStatus(newStatus)
	newInterval := pollIntervalForTier(newTier)
	resourceVersion := strings.TrimSpace(liveVM.ResourceVersion)

	updated := *vmRow
	updated.Status = newStatus
	updated.PollingTier = newTier
	updated.PollIntervalSec = newInterval

	var highTierSince *time.Time
	switch newTier {
	case entvm.PollingTierHigh:
		if vmRow.HighTierSince != nil {
			highTierSince = vmRow.HighTierSince
		} else {
			ts := observedAt
			highTierSince = &ts
		}
	default:
		highTierSince = nil
	}
	updated.HighTierSince = highTierSince

	ts := observedAt
	updated.LastPolledAt = &ts
	if resourceVersion != "" {
		rv := resourceVersion
		updated.LastK8sRv = &rv
	}

	if !shouldPersistObservedVMState(vmRow, updated, resourceVersion, observedAt) {
		return &updated
	}

	update := s.client.VM.UpdateOneID(vmRow.ID).
		SetStatus(newStatus).
		SetPollingTier(newTier).
		SetPollIntervalSec(newInterval)

	if highTierSince == nil {
		update = update.ClearHighTierSince()
	} else {
		update = update.SetHighTierSince(*highTierSince)
	}

	if vmRow.LastPolledAt == nil || observedAt.Sub(*vmRow.LastPolledAt) >= liveVMStatusPersistMinInterval {
		update = update.SetLastPolledAt(observedAt)
	}
	if resourceVersion != "" {
		update = update.SetLastK8sRv(resourceVersion)
	}

	if _, err := update.Save(ctx); err != nil {
		logger.Warn("failed to persist observed vm state",
			zap.String("vm_id", vmRow.ID),
			zap.String("cluster_id", vmRow.ClusterID),
			zap.String("namespace", vmRow.Namespace),
			zap.String("name", vmRow.Name),
			zap.String("status", string(newStatus)),
			zap.Error(err),
		)
	}

	return &updated
}

// applyUnavailableVMState handles VMs that are not visible on K8s.
// isNotFound=true  → cluster responded OK but VM resource is gone → NOT_FOUND
// isNotFound=false → K8s API call failed (cluster unreachable)   → UNKNOWN
func (s *Server) applyUnavailableVMState(ctx context.Context, vmRow *ent.VM, observedAt time.Time, isNotFound bool) *ent.VM {
	if s == nil || s.client == nil || vmRow == nil {
		return vmRow
	}

	newStatus := reconcileUnavailableVMStatus(vmRow, observedAt, isNotFound)
	newTier := pollingTierForVMStatus(newStatus)
	newInterval := pollIntervalForTier(newTier)

	updated := *vmRow
	updated.Status = newStatus
	updated.PollingTier = newTier
	updated.PollIntervalSec = newInterval
	updated.LastPolledAt = &observedAt
	updated.LastK8sRv = nil
	if newTier == entvm.PollingTierHigh {
		if vmRow.HighTierSince == nil {
			ts := observedAt
			updated.HighTierSince = &ts
		}
	} else {
		updated.HighTierSince = nil
	}

	if !shouldPersistUnavailableVMState(vmRow, updated, observedAt) {
		return &updated
	}

	update := s.client.VM.UpdateOneID(vmRow.ID).
		SetStatus(newStatus).
		SetPollingTier(newTier).
		SetPollIntervalSec(newInterval).
		ClearLastK8sRv()

	if updated.HighTierSince == nil {
		update = update.ClearHighTierSince()
	} else {
		update = update.SetHighTierSince(*updated.HighTierSince)
	}
	if vmRow.LastPolledAt == nil || observedAt.Sub(*vmRow.LastPolledAt) >= liveVMStatusPersistMinInterval {
		update = update.SetLastPolledAt(observedAt)
	}

	if _, err := update.Save(ctx); err != nil {
		logger.Warn("failed to persist unavailable vm state",
			zap.String("vm_id", vmRow.ID),
			zap.String("cluster_id", vmRow.ClusterID),
			zap.String("namespace", vmRow.Namespace),
			zap.String("name", vmRow.Name),
			zap.String("status", string(newStatus)),
			zap.Error(err),
		)
	}

	return &updated
}

func shouldPersistObservedVMState(vmRow *ent.VM, updated ent.VM, resourceVersion string, observedAt time.Time) bool {
	if vmRow.Status != updated.Status {
		return true
	}
	if vmRow.PollingTier != updated.PollingTier {
		return true
	}
	if vmRow.PollIntervalSec != updated.PollIntervalSec {
		return true
	}
	if (vmRow.HighTierSince == nil) != (updated.HighTierSince == nil) {
		return true
	}
	if vmRow.HighTierSince != nil && updated.HighTierSince != nil && !vmRow.HighTierSince.Equal(*updated.HighTierSince) {
		return true
	}
	if resourceVersion != "" {
		if vmRow.LastK8sRv == nil || strings.TrimSpace(*vmRow.LastK8sRv) != resourceVersion {
			return true
		}
	}
	return vmRow.LastPolledAt == nil || observedAt.Sub(*vmRow.LastPolledAt) >= liveVMStatusPersistMinInterval
}

func shouldPersistUnavailableVMState(vmRow *ent.VM, updated ent.VM, observedAt time.Time) bool {
	if vmRow.Status != updated.Status {
		return true
	}
	if vmRow.PollingTier != updated.PollingTier {
		return true
	}
	if vmRow.PollIntervalSec != updated.PollIntervalSec {
		return true
	}
	if vmRow.LastK8sRv != nil {
		return true
	}
	if (vmRow.HighTierSince == nil) != (updated.HighTierSince == nil) {
		return true
	}
	if vmRow.HighTierSince != nil && updated.HighTierSince != nil && !vmRow.HighTierSince.Equal(*updated.HighTierSince) {
		return true
	}
	return vmRow.LastPolledAt == nil || observedAt.Sub(*vmRow.LastPolledAt) >= liveVMStatusPersistMinInterval
}

func reconcileUnavailableVMStatus(vmRow *ent.VM, observedAt time.Time, isNotFound bool) entvm.Status {
	if shouldHoldCreateBootstrapStatus(vmRow, entvm.StatusUNKNOWN, observedAt) {
		return vmRow.Status
	}
	if isNotFound {
		return entvm.StatusNOT_FOUND
	}
	return entvm.StatusUNKNOWN
}

func shouldHoldCreateBootstrapStatus(vmRow *ent.VM, observed entvm.Status, observedAt time.Time) bool {
	if vmRow == nil {
		return false
	}
	if vmRow.PollingTier != entvm.PollingTierHigh {
		return false
	}
	if vmRow.Status != entvm.StatusCREATING && vmRow.Status != entvm.StatusSTARTING && vmRow.Status != entvm.StatusRUNNING {
		return false
	}
	if observed != entvm.StatusSTOPPED && observed != entvm.StatusUNKNOWN {
		return false
	}
	if vmRow.CreatedAt.IsZero() {
		return false
	}
	return observedAt.Sub(vmRow.CreatedAt) <= liveVMCreateBootstrapGraceWindow
}

func mapDomainVMStatusToEntVM(status domain.VMStatus) entvm.Status {
	switch status {
	case domain.VMStatusCreating:
		return entvm.StatusCREATING
	case domain.VMStatusStarting:
		return entvm.StatusSTARTING
	case domain.VMStatusRunning:
		return entvm.StatusRUNNING
	case domain.VMStatusStopping:
		return entvm.StatusSTOPPING
	case domain.VMStatusStopped:
		return entvm.StatusSTOPPED
	case domain.VMStatusDeleting:
		return entvm.StatusDELETING
	case domain.VMStatusFailed:
		return entvm.StatusFAILED
	case domain.VMStatusPending:
		return entvm.StatusPENDING
	case domain.VMStatusMigrating:
		return entvm.StatusMIGRATING
	case domain.VMStatusPaused:
		return entvm.StatusPAUSED
	default:
		return entvm.StatusUNKNOWN
	}
}

func pollingTierForVMStatus(status entvm.Status) entvm.PollingTier {
	switch status {
	case entvm.StatusCREATING, entvm.StatusSTARTING, entvm.StatusDELETING, entvm.StatusSTOPPING, entvm.StatusMIGRATING, entvm.StatusPENDING:
		return entvm.PollingTierHigh
	default:
		return entvm.PollingTierLow
	}
}

func pollIntervalForTier(tier entvm.PollingTier) int {
	switch tier {
	case entvm.PollingTierHigh:
		return 15
	default:
		return 1800
	}
}
