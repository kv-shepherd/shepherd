import type { VMPlacementHint } from './types';

type PlacementReasonCount = NonNullable<VMPlacementHint['reason_counts']>[number];
type PlacementAdvisoryCount = NonNullable<VMPlacementHint['advisory_counts']>[number];

export function sortPlacementReasonCounts(reasonCounts: VMPlacementHint['reason_counts']): PlacementReasonCount[] {
    return [...(reasonCounts ?? [])].sort((left, right) => {
        if (right.count !== left.count) {
            return right.count - left.count;
        }
        return left.code.localeCompare(right.code);
    });
}

export function sortPlacementAdvisoryCounts(advisoryCounts: VMPlacementHint['advisory_counts']): PlacementAdvisoryCount[] {
    return [...(advisoryCounts ?? [])].sort((left, right) => {
        if (right.count !== left.count) {
            return right.count - left.count;
        }
        return left.code.localeCompare(right.code);
    });
}

export function getPlacementReasonActionKey(reasonCode: VMPlacementHint['primary_reason_code']): string {
    switch (reasonCode) {
        case 'NoCandidateClusters':
            return 'wizard.placement_action.NoCandidateClusters';
        case 'ClusterUnavailable':
            return 'wizard.placement_action.ClusterUnavailable';
        case 'CapabilityMismatch':
            return 'wizard.placement_action.CapabilityMismatch';
        case 'PolicyNotConfigured':
            return 'wizard.placement_action.PolicyNotConfigured';
        case 'PolicyDenied':
            return 'wizard.placement_action.PolicyDenied';
        case 'RequestInvalid':
            return 'wizard.placement_action.RequestInvalid';
        default:
            return 'wizard.placement_action.Other';
    }
}

export function getPlacementAdvisoryLabelKey(advisoryCode: VMPlacementHint['primary_advisory_code']): string {
    switch (advisoryCode) {
        case 'HostAssistedCloneLikely':
            return 'wizard.placement_advisory.HostAssistedCloneLikely';
        default:
            return 'wizard.placement_advisory.Other';
    }
}
