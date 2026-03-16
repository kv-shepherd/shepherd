import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { MockInstance } from 'vitest';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { useApiGetMock, useApiMutationMock, useApiActionMock } = vi.hoisted(() => ({
    useApiGetMock: vi.fn(),
    useApiMutationMock: vi.fn(),
    useApiActionMock: vi.fn(),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, fallback?: string | { defaultValue?: string }) => {
            if (typeof fallback === 'string') {
                return fallback;
            }
            return fallback?.defaultValue ?? key;
        },
    }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
    useApiMutation: (...args: unknown[]) => useApiMutationMock(...args),
    useApiAction: (...args: unknown[]) => useApiActionMock(...args),
}));

import { AdminTemplatesContent } from './AdminTemplatesContent';
import type { Template } from '../types';

function buildTemplate(overrides: Partial<Template> = {}): Template {
    return {
        id: 'tpl-1',
        name: 'fedora-eval',
        display_name: 'Fedora Eval',
        description: 'Fedora base template',
        catalog_scope: 'test',
        source_type: 'cdi_image_import',
        image_url: 'docker://quay.io/containerdisks/fedora:latest',
        pvc_name: undefined,
        pvc_namespace: undefined,
        cloud_init: '#cloud-config\nhostname: fedora-eval',
        os_family: 'linux',
        os_version: 'Fedora',
        enabled: true,
        created_at: '2026-03-16T00:00:00Z',
        ...overrides,
    };
}

describe('AdminTemplatesContent', () => {
    let consoleErrorSpy: MockInstance<typeof console.error>;
    let createMutate: ReturnType<typeof vi.fn>;
    let updateMutate: ReturnType<typeof vi.fn>;
    let deleteMutate: ReturnType<typeof vi.fn>;
    let mutationCallIndex: number;

    beforeEach(() => {
        vi.clearAllMocks();
        consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
        createMutate = vi.fn();
        updateMutate = vi.fn();
        deleteMutate = vi.fn();
        mutationCallIndex = 0;

        useApiGetMock.mockReturnValue({
            data: {
                items: [],
                pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
            },
            isLoading: false,
            refetch: vi.fn(),
        });
        useApiMutationMock.mockImplementation(() => {
            const next = mutationCallIndex % 2 === 0
                ? { mutate: createMutate, isPending: false }
                : { mutate: updateMutate, isPending: false };
            mutationCallIndex += 1;
            return next;
        });
        useApiActionMock.mockReturnValue({
            mutate: deleteMutate,
            isPending: false,
        });
    });

    it('opens the create modal without disconnected form warnings', async () => {
        const user = userEvent.setup();
        render(<AdminTemplatesContent />);

        await user.click(screen.getByTestId('template-create-button'));

        expect(await screen.findByPlaceholderText('centos7-standard')).toBeInTheDocument();
        expect(
            consoleErrorSpy.mock.calls.some((call) =>
                call.some((value) => String(value).includes('Instance created by `useForm` is not connected to any Form element')),
            ),
        ).toBe(false);
        expect(
            consoleErrorSpy.mock.calls.some((call) =>
                call.some((value) => String(value).includes('There may be circular references')),
            ),
        ).toBe(false);
    });

    it('hydrates the official fedora preset into the create form', async () => {
        const user = userEvent.setup();
        render(<AdminTemplatesContent />);

        await user.click(screen.getByTestId('template-create-button'));
        await user.click(screen.getByRole('button', { name: 'templates.official_preset_fedora_eval' }));

        expect(await screen.findByText('Fedora')).toBeInTheDocument();
        expect(screen.getByText('docker://quay.io/containerdisks/fedora:latest')).toBeInTheDocument();
        expect(screen.getByDisplayValue(/#cloud-config/)).toBeInTheDocument();
    }, 20000);

    it('hydrates the curated linux prod preset into the create form', async () => {
        const user = userEvent.setup();
        render(<AdminTemplatesContent />);

        await user.click(screen.getByTestId('template-create-button'));
        await user.click(screen.getByRole('button', { name: 'templates.preset_linux_prod' }));

        expect(await screen.findByText('Kylin V10')).toBeInTheDocument();
        expect(screen.getByText('vm-muban')).toBeInTheDocument();
        expect(screen.getByText('kylinv10-image')).toBeInTheDocument();
        expect(screen.getByDisplayValue(/#cloud-config/)).toBeInTheDocument();
    }, 20000);

    it('hydrates edit values for pvc clone templates after the modal opens', async () => {
        const user = userEvent.setup();
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    buildTemplate({
                        id: 'tpl-pvc',
                        name: 'kylin-prod',
                        display_name: 'Kylin V10',
                        catalog_scope: 'prod',
                        source_type: 'cdi_pvc_clone',
                        image_url: undefined,
                        pvc_namespace: 'vm-muban',
                        pvc_name: 'kylinv10-image',
                        cloud_init: '#cloud-config\nhostname: kylin-prod',
                        os_version: 'Kylin V10',
                    }),
                ],
                pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
            },
            isLoading: false,
            refetch: vi.fn(),
        });

        render(<AdminTemplatesContent />);

        await user.click(screen.getByTestId('template-action-edit-tpl-pvc'));

        expect(await screen.findByText('vm-muban')).toBeInTheDocument();
        expect(screen.getByText('kylinv10-image')).toBeInTheDocument();
        expect(screen.getByDisplayValue(/#cloud-config/)).toBeInTheDocument();
        expect(
            consoleErrorSpy.mock.calls.some((call) =>
                call.some((value) => String(value).includes('Instance created by `useForm` is not connected to any Form element')),
            ),
        ).toBe(false);
    }, 20000);
});
