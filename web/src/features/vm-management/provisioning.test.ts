import { describe, expect, it } from 'vitest';

import {
    hasProvisioningFailure,
    hasVisibleProvisioningStatus,
    isVMProvisioningPollingActive,
    parseProvisioningPercent,
} from './provisioning';

describe('provisioning helpers', () => {
    it('parses CDI-style percentage strings conservatively', () => {
        expect(parseProvisioningPercent('75.4%')).toBe(75);
        expect(parseProvisioningPercent('100%')).toBe(100);
        expect(parseProvisioningPercent('120%')).toBe(100);
        expect(parseProvisioningPercent('-1%')).toBeUndefined();
        expect(parseProvisioningPercent('waiting')).toBeUndefined();
    });

    it('keeps VM detail polling active only while provisioning is non-terminal', () => {
        expect(isVMProvisioningPollingActive({ status: 'CREATING' })).toBe(true);
        expect(isVMProvisioningPollingActive({
            status: 'RUNNING',
            provisioning: { phase: 'ImportInProgress' },
        })).toBe(true);
        expect(isVMProvisioningPollingActive({
            status: 'RUNNING',
            provisioning: { phase: 'Succeeded' },
        })).toBe(false);
        expect(isVMProvisioningPollingActive({
            status: 'CREATING',
            provisioning: { phase: 'Failed', failure_message: 'quota exceeded' },
        })).toBe(false);
    });

    it('treats only an explicit failed phase as provisioning failure', () => {
        expect(hasProvisioningFailure({
            phase: 'Succeeded',
            failure_message: 'Clone Complete',
        })).toBe(false);
        expect(hasProvisioningFailure({
            phase: 'Failed',
            failure_message: 'quota exceeded',
        })).toBe(true);
    });

    it('detects visible provisioning telemetry', () => {
        expect(hasVisibleProvisioningStatus(undefined)).toBe(false);
        expect(hasVisibleProvisioningStatus({})).toBe(false);
        expect(hasVisibleProvisioningStatus({ claim_name: 'root-dv' })).toBe(true);
        expect(hasVisibleProvisioningStatus({ recent_events: [{ reason: 'Pulling' }] })).toBe(true);
    });
});
