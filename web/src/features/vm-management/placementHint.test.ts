import { describe, expect, it } from 'vitest';

import {
    getPlacementAdvisoryLabelKey,
    getPlacementReasonActionKey,
    sortPlacementAdvisoryCounts,
    sortPlacementReasonCounts,
} from './placementHint';

describe('placementHint helpers', () => {
    it('sorts reason counts by count descending and code ascending for ties', () => {
        expect(
            sortPlacementReasonCounts([
                { code: 'PolicyDenied', count: 1 },
                { code: 'CapabilityMismatch', count: 3 },
                { code: 'ClusterUnavailable', count: 3 },
            ]),
        ).toEqual([
            { code: 'CapabilityMismatch', count: 3 },
            { code: 'ClusterUnavailable', count: 3 },
            { code: 'PolicyDenied', count: 1 },
        ]);
    });

    it('maps primary reason codes to stable action translation keys', () => {
        expect(getPlacementReasonActionKey('PolicyDenied')).toBe('wizard.placement_action.PolicyDenied');
        expect(getPlacementReasonActionKey('CapabilityMismatch')).toBe('wizard.placement_action.CapabilityMismatch');
        expect(getPlacementReasonActionKey(undefined)).toBe('wizard.placement_action.Other');
    });

    it('sorts advisory counts and maps advisory labels to stable translation keys', () => {
        expect(
            sortPlacementAdvisoryCounts([
                { code: 'Other', count: 1 },
                { code: 'HostAssistedCloneLikely', count: 2 },
            ]),
        ).toEqual([
            { code: 'HostAssistedCloneLikely', count: 2 },
            { code: 'Other', count: 1 },
        ]);
        expect(getPlacementAdvisoryLabelKey('HostAssistedCloneLikely')).toBe('wizard.placement_advisory.HostAssistedCloneLikely');
        expect(getPlacementAdvisoryLabelKey(undefined)).toBe('wizard.placement_advisory.Other');
    });
});
