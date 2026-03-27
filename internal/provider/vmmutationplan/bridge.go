package vmmutationplan

import (
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
)

type VMLiveUpdateTargets = provider.VMLiveUpdateTargets

type VMResourceUpdatePlan = provider.VMResourceUpdatePlan

func PlanVMResourceUpdatePatch(namespace string, current *domain.VM, target VMLiveUpdateTargets) (*VMResourceUpdatePlan, error) {
	return provider.PlanVMResourceUpdatePatch(namespace, current, target)
}
