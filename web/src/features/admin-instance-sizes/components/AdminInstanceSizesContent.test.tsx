import { render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { forwardRef, useImperativeHandle } from 'react';

const controllerState = vi.hoisted(() => ({
    createOpen: false,
    editOpen: false,
    editingItem: null as Record<string, unknown> | null,
    editInitialValues: undefined as Record<string, unknown> | undefined,
}));

vi.mock('next/navigation', () => ({
    useRouter: () => ({
        push: vi.fn(),
    }),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, fallback?: string | { defaultValue?: string; value?: string }) => {
            if (typeof fallback === 'string') {
                return fallback;
            }
            if (fallback?.value) {
                return `${key} ${fallback.value}`;
            }
            return fallback?.defaultValue ?? key;
        },
    }),
}));

vi.mock('@/features/setup-guide/hooks/useSetupGuide', () => ({
    useSetupGuide: () => ({
        systemsTotal: 1,
        servicesTotal: 1,
        vmsTotal: 0,
        namespacesTotal: 1,
        templatesTotal: 1,
        instanceSizesTotal: 1,
        canCreateSystem: true,
        canCreateService: true,
        canCreateVM: true,
        canManageNamespaces: true,
        canManageTemplates: true,
        canManageInstanceSizes: true,
        systemReady: true,
        serviceReady: true,
        prerequisitesReady: true,
        vmRequestReady: true,
        hasRequestedFirstVM: false,
        isLoading: false,
    }),
}));

vi.mock('../hooks/useAdminInstanceSizesController', async () => {
    const { Form } = await import('antd');
    return {
        useAdminInstanceSizesController: () => {
            const [createForm] = Form.useForm();
            const [editForm] = Form.useForm();
            return {
                messageContextHolder: null,
                filters: {
                    search: '',
                    catalogScope: '',
                    enabled: '',
                    publication: '',
                    capability: '',
                },
                hasActiveFilters: false,
                applyFilters: vi.fn(),
                clearFilters: vi.fn(),
                globalSearch: '',
                setGlobalSearch: vi.fn(),
                deferredSearch: '',
                isStale: false,
                searchedColumn: '',
                setSearchedColumn: vi.fn(),
                searchText: '',
                setSearchText: vi.fn(),
                filteredItems: [
                    {
                        id: 'size-1',
                        name: 'm4.large',
                        display_name: '',
                        description: 'general purpose',
                        catalog_scope: 'all',
                        cpu_cores: 4,
                        memory_gi: 8,
                        disk_gb: 80,
                        cpu_request: 2,
                        memory_request_gi: 6,
                        requires_gpu: true,
                        spec_overrides: {
                            spec: {
                                template: {
                                    spec: {
                                        domain: {
                                            devices: {
                                                gpus: [{ name: 'gpu0', deviceName: 'nvidia.com/A10' }],
                                            },
                                        },
                                    },
                                },
                            },
                        },
                        enabled: true,
                    },
                ],
                data: { items: [] },
                isLoading: false,
                refetch: vi.fn(),
                createOpen: controllerState.createOpen,
                editOpen: controllerState.editOpen,
                deleteOpen: false,
                editingItem: controllerState.editingItem,
                deletingItem: null,
                createForm,
                editForm,
                createInitialValues: {
                    catalog_scope: 'unclassified',
                    enabled: true,
                    sort_order: 0,
                    dedicated_cpu: false,
                    spec_text: '{}',
                    root_volume_mode_intent: 'auto',
                },
                editInitialValues: controllerState.editInitialValues,
                openCreateModal: vi.fn(),
                openEditModal: vi.fn(),
                openDeleteModal: vi.fn(),
                closeCreateModal: vi.fn(),
                closeEditModal: vi.fn(),
                closeDeleteModal: vi.fn(),
                submitCreate: vi.fn(),
                submitEdit: vi.fn(),
                submitDelete: vi.fn(),
                createPending: false,
                updatePending: false,
                deletePending: false,
            };
        },
    };
});

vi.mock('../../admin-templates/hooks/useDynamicSchema', () => ({
    useDynamicSchema: () => ({
        data: {
            version: 1,
            entity_type: 'instancesize',
            schema: { type: 'object', properties: {} },
            ui_mask: [],
            generated_at: '2026-03-11T00:00:00Z',
        },
        isLoading: false,
    }),
}));

vi.mock('../../admin-templates/components/DynamicSchemaForm', () => ({
    DynamicSchemaForm: (() => {
        const DynamicSchemaFormMock = forwardRef(function DynamicSchemaFormMock(_props: unknown, ref) {
        useImperativeHandle(ref, () => ({
            sync: vi.fn(),
        }));
        return <div data-testid="dynamic-schema-form" />;
        });
        DynamicSchemaFormMock.displayName = 'DynamicSchemaFormMock';
        return DynamicSchemaFormMock;
    })(),
}));

import { AdminInstanceSizesContent, applyInstanceSizePreset } from './AdminInstanceSizesContent';

describe('AdminInstanceSizesContent', () => {
    beforeEach(() => {
        controllerState.createOpen = false;
        controllerState.editOpen = false;
        controllerState.editingItem = null;
        controllerState.editInitialValues = undefined;
    });

    it('pre-mounts the edit dynamic schema form so first-open hydration has a registered Form tree', () => {
        render(<AdminInstanceSizesContent />);

        expect(screen.getAllByTestId('dynamic-schema-form')).toHaveLength(1);
    });

    it('renders the create form without duplicate initialValues warnings', async () => {
        controllerState.createOpen = true;
        const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

        render(<AdminInstanceSizesContent />);

        expect(await screen.findByTestId('instance-size-create-modal')).toBeInTheDocument();
        expect(
            consoleErrorSpy.mock.calls.some((call) =>
                call.some((value) => String(value).includes("Field can not overwrite it")),
            ),
        ).toBe(false);

        consoleErrorSpy.mockRestore();
        controllerState.createOpen = false;
    });

    it('hydrates dedicated cpu in the edit modal without re-enabling cpu overcommit', async () => {
        controllerState.editOpen = true;
        controllerState.editingItem = {
            id: 'size-dedicated',
            name: 'm4.dedicated',
        };
        controllerState.editInitialValues = {
            name: 'm4.dedicated',
            catalog_scope: 'prod',
            cpu_cores: 4,
            memory_gi: 8,
            cpu_request: 4,
            dedicated_cpu: true,
            cpu_overcommit_enabled: false,
            memory_overcommit_enabled: false,
            enabled: true,
            spec_text: JSON.stringify({
                spec: {
                    template: {
                        spec: {
                            domain: {
                                cpu: {
                                    dedicatedCpuPlacement: true,
                                },
                            },
                        },
                    },
                },
            }, null, 2),
        };

        render(<AdminInstanceSizesContent />);

        expect(await screen.findByTestId('instance-size-edit-modal')).toBeInTheDocument();

        const dedicatedLabel = screen.getByText('instanceSizes.dedicated').closest('label');
        const dedicatedCheckbox = within(dedicatedLabel as HTMLElement).getByRole('checkbox') as HTMLInputElement;
        expect(dedicatedCheckbox).toBeChecked();

        const overcommitLabel = screen.getByText('instanceSizes.enable_cpu_overcommit').closest('label');
        const overcommitCheckbox = within(overcommitLabel as HTMLElement).getByRole('checkbox') as HTMLInputElement;
        expect(overcommitCheckbox).not.toBeChecked();
        expect(overcommitCheckbox).toBeDisabled();
    });

    it('writes preset request values on the first apply', () => {
        const setFieldsValue = vi.fn();
        const mockForm = {
            getFieldsValue: vi.fn(() => ({
                name: 'kept-name',
                display_name: 'Kept Display Name',
                description: 'Kept Description',
                sort_order: 9,
            })),
            setFieldsValue,
        };

        applyInstanceSizePreset(
            mockForm as never,
            { current: null },
            'linux-test',
        );

        expect(setFieldsValue).toHaveBeenCalledTimes(1);
        expect(setFieldsValue).toHaveBeenCalledWith(expect.objectContaining({
            name: 'kept-name',
            display_name: 'Kept Display Name',
            description: 'Kept Description',
            sort_order: 9,
            cpu_overcommit_enabled: true,
            cpu_request: 2,
            memory_overcommit_enabled: true,
            memory_request_gi: 4,
            root_volume_mode_intent: 'auto',
        }));
    });

    it('falls back to name when display_name is an empty string', async () => {
        render(<AdminInstanceSizesContent />);

        await screen.findByTestId('instance-size-action-edit-size-1');
        const nameCells = screen.getAllByText('m4.large');
        expect(nameCells).toHaveLength(2);
    });

    it('renders overcommit request values and gpu device labels', async () => {
        render(<AdminInstanceSizesContent />);

        await screen.findByText((content) => content.includes('instanceSizes.request_compact 2 instanceSizes.cores'));
        expect(
            screen.getByText((content) => content.includes('instanceSizes.request_compact 2 instanceSizes.cores')),
        ).toBeInTheDocument();
        expect(
            screen.getByText((content) => content.includes('instanceSizes.request_compact 6 Gi')),
        ).toBeInTheDocument();
        expect(screen.getByText('GPU nvidia.com/A10')).toBeInTheDocument();
    });
});
