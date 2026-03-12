import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { forwardRef, useImperativeHandle } from 'react';

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

vi.mock('../hooks/useAdminInstanceSizesController', async () => {
    const { Form } = await import('antd');
    return {
        useAdminInstanceSizesController: () => {
            const [createForm] = Form.useForm();
            const [editForm] = Form.useForm();
            return {
                messageContextHolder: null,
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
                createOpen: false,
                editOpen: false,
                deleteOpen: false,
                editingItem: null,
                deletingItem: null,
                createForm,
                editForm,
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

import { AdminInstanceSizesContent } from './AdminInstanceSizesContent';

describe('AdminInstanceSizesContent', () => {
    it('falls back to name when display_name is an empty string', async () => {
        render(<AdminInstanceSizesContent />);

        await screen.findByTestId('instance-size-action-edit-size-1');
        const nameCells = screen.getAllByText('m4.large');
        expect(nameCells).toHaveLength(2);
    }, 15000);

    it('renders overcommit request values and gpu device labels', async () => {
        render(<AdminInstanceSizesContent />);

        await screen.findByText('instanceSizes.request_compact 2 instanceSizes.cores');
        expect(screen.getByText('instanceSizes.request_compact 2 instanceSizes.cores')).toBeInTheDocument();
        expect(screen.getByText('instanceSizes.request_compact 6 Gi')).toBeInTheDocument();
        expect(screen.getByText('GPU nvidia.com/A10')).toBeInTheDocument();
    });
});
