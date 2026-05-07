import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (
            key: string,
            options?: { defaultValue?: string; phase?: string; progress?: string },
        ) => {
            const labels: Record<string, string> = {
                'provisioning.current_title': 'Provisioning telemetry',
                'provisioning.failed_title': 'Provisioning failed',
                'provisioning.failed_without_message': 'Provisioning failed without message',
                'provisioning.phase': 'Phase',
                'provisioning.phase_unknown': 'unknown',
                'provisioning.progress': 'Progress',
                'provisioning.progress_unknown': 'unknown',
                'provisioning.root_claim': 'Root Claim',
                'provisioning.pvc_phase': 'PVC Phase',
                'provisioning.clone_type': 'Clone Type',
                'provisioning.clone_type_copy': 'Host-assisted copy',
                'provisioning.restart_count': 'Restarts',
                'provisioning.clone_fallback_reason': 'Clone fallback reason',
                'provisioning.conditions': 'Conditions',
                'provisioning.recent_events': 'Recent Events',
            };
            if (key === 'provisioning.current_description') {
                return `Phase ${options?.phase ?? ''}, progress ${options?.progress ?? ''}.`;
            }
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

import { VMProvisioningStatusPanel } from './VMProvisioningStatusPanel';

describe('VMProvisioningStatusPanel', () => {
    it('renders active provisioning progress, clone fallback, conditions, and events', () => {
        render(
            <VMProvisioningStatusPanel
                pollingActive={true}
                provisioning={{
                    phase: 'ImportInProgress',
                    progress: '75.0%',
                    claim_name: 'root-dv',
                    pvc_phase: 'Bound',
                    clone_type: 'copy',
                    clone_fallback_reason: 'StorageProfile did not allow smart clone',
                    restart_count: 1,
                    conditions: [
                        {
                            type: 'DataVolumeReady',
                            status: 'False',
                            reason: 'Importing',
                            message: 'Importing root disk',
                            last_transition_time: '2026-03-17T00:00:00Z',
                        },
                    ],
                    recent_events: [
                        {
                            type: 'Normal',
                            reason: 'ImportScheduled',
                            message: 'Importer pod scheduled',
                            count: 2,
                            last_observed: '2026-03-17T00:01:00Z',
                        },
                    ],
                }}
            />,
        );

        expect(screen.getByTestId('vm-provisioning-panel')).toBeVisible();
        expect(screen.getByTestId('vm-provisioning-phase')).toHaveTextContent('ImportInProgress');
        expect(screen.getByTestId('vm-provisioning-progress')).toHaveTextContent('75%');
        expect(screen.getByText('root-dv')).toBeVisible();
        expect(screen.getByText('Host-assisted copy')).toBeVisible();
        expect(screen.getByText('StorageProfile did not allow smart clone')).toBeVisible();
        expect(screen.getByText('Importing root disk')).toBeVisible();
        expect(screen.getByText('Importer pod scheduled')).toBeVisible();
    });

    it('surfaces provisioning failure details', () => {
        render(
            <VMProvisioningStatusPanel
                provisioning={{
                    phase: 'Failed',
                    progress: '40%',
                    failure_message: 'quota exceeded',
                }}
            />,
        );

        expect(screen.getByText('Provisioning failed')).toBeVisible();
        expect(screen.getByText('quota exceeded')).toBeVisible();
    });

    it('does not style non-failed phases as failures when a historical message is present', () => {
        render(
            <VMProvisioningStatusPanel
                provisioning={{
                    phase: 'Succeeded',
                    progress: '100%',
                    failure_message: 'Clone Complete',
                }}
            />,
        );

        expect(screen.getByText('Provisioning telemetry')).toBeVisible();
        expect(screen.queryByText('Provisioning failed')).not.toBeInTheDocument();
    });
});
