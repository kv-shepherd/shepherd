'use client';

import { useRef, type RefObject } from 'react';
import {
    Alert,
    Button,
    Card,
    Checkbox,
    Divider,
    Empty,
    Form,
    Input,
    InputNumber,
    Modal,
    Select,
    Space,
    Spin,
    Switch,
    Table,
    Tag,
    Tooltip,
    Typography,
    type FormInstance,
} from 'antd';
import type { InputRef } from 'antd';
import type { ColumnsType, FilterDropdownProps } from 'antd/es/table/interface';
import {
    DeleteOutlined,
    EditOutlined,
    HddOutlined,
    ReloadOutlined,
    SearchOutlined,
    ThunderboltOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { UnitInputNumber } from '@/components/form/UnitInputNumber';
import { useAdminInstanceSizesController } from '../hooks/useAdminInstanceSizesController';
import {
    formatCores,
    formatMemory,
    getGPUDeviceLabels,
    hasCPUOvercommit,
    hasMemoryOvercommit,
    type InstanceSize,
} from '../types';
import {
    DynamicSchemaForm,
    type DynamicSchemaFormHandle,
    type SchemaNode,
    type SchemaMask,
} from '../../admin-templates/components/DynamicSchemaForm';
import { useDynamicSchema } from '../../admin-templates/hooks/useDynamicSchema';

const { Title, Text } = Typography;

function catalogScopeLabel(scope: InstanceSize['catalog_scope'], t: ReturnType<typeof useTranslation>['t']): string {
    switch (scope) {
        case 'test':
            return t('templates.scope_test');
        case 'prod':
            return t('templates.scope_prod');
        case 'all':
            return t('templates.scope_all');
        default:
            return t('templates.scope_unclassified');
    }
}

function catalogScopeColor(scope: InstanceSize['catalog_scope']): string {
    switch (scope) {
        case 'test':
            return 'blue';
        case 'prod':
            return 'red';
        case 'all':
            return 'green';
        default:
            return 'default';
    }
}

function handleInstanceSizeFormValuesChange(
    form: FormInstance,
    formRef: RefObject<DynamicSchemaFormHandle | null>,
    changedValues: Record<string, unknown>,
) {
    const updates: Record<string, unknown> = {};

    if (changedValues.dedicated_cpu === true) {
        updates.cpu_overcommit_enabled = false;
        updates.cpu_request = undefined;
    }
    if (changedValues.cpu_overcommit_enabled === true) {
        updates.dedicated_cpu = false;
    }
    if (changedValues.cpu_overcommit_enabled === false) {
        updates.cpu_request = undefined;
    }
    if (changedValues.memory_overcommit_enabled === false) {
        updates.memory_request_gi = undefined;
    }

    if (Object.keys(updates).length > 0) {
        form.setFieldsValue(updates);
    }
    formRef.current?.sync();
}

function highlightText(text: string, highlight: string): React.ReactNode {
    if (!highlight) {
        return text;
    }
    const index = text.toLowerCase().indexOf(highlight.toLowerCase());
    if (index === -1) {
        return text;
    }
    const before = text.slice(0, index);
    const match = text.slice(index, index + highlight.length);
    const after = text.slice(index + highlight.length);
    return (
        <>
            {before}
            <span style={{ backgroundColor: '#ffc069', fontWeight: 600, padding: '0 2px' }}>
                {match}
            </span>
            {after}
        </>
    );
}

/**
 * Schema-driven spec_overrides section for InstanceSize modals.
 *
 * Replaces hardcoded Hugepages Select + GPU Form.List + JSON textarea.
 * Loads schema+mask from GET /schemas/instancesize (ADR-0023 Stage 1).
 *
 * Degradation chain (ADR-0023):
 *   success → DynamicSchemaForm renders mask fields
 *   isError → collapsed warning, form still submittable without dynamic fields
 *   isPending → loading spinner
 */
function InstanceSizeSpecSection({
    formRef,
    disabled,
}: {
    formRef: React.RefObject<DynamicSchemaFormHandle | null>;
    disabled?: boolean;
}) {
    const { t } = useTranslation(['admin', 'common']);
    const { data, isError, isPending } = useDynamicSchema('instancesize');

    if (isPending) {
        return (
            <Card size="small" style={{ textAlign: 'center', padding: 24 }}>
                <Spin size="small" />
                <Text type="secondary" style={{ marginLeft: 8 }}>
                    {t('instanceSizes.schema_loading', 'Loading spec schema…')}
                </Text>
            </Card>
        );
    }

    if (isError || !data) {
        return (
            <Alert
                type="warning"
                showIcon
                message={t(
                    'instanceSizes.schema_unavailable',
                    'Spec schema unavailable — advanced KubeVirt settings cannot be configured right now.'
                )}
                style={{ marginBottom: 8 }}
            />
        );
    }

    return (
        <Form.Item
            name="spec_text"
            // valuePropName="value" is injected by Form.Item automatically
            noStyle
        >
            <DynamicSchemaForm
                ref={formRef}
                schema={data.schema as SchemaNode}
                mask={data.mask as SchemaMask}
                disabled={disabled}
            />
        </Form.Item>
    );
}

/**
 * Shared form fields for InstanceSize create/edit modals.
 *
 * Architecture:
 * - Metadata fields (cpu_cores, memory_gi, overcommit, etc.) remain as
 *   explicit Ant Design Form.Items — these are InstanceSize-specific API fields.
 * - Spec overrides (hugepages, GPU devices, CPU model, etc.) are rendered
 *   schema-driven via DynamicSchemaForm (ADR-0023 Stage 1).
 *
 * The `formRef` is passed down so the parent Form's onValuesChange can
 * call formRef.current?.sync() to keep spec_text in sync (antd best practice:
 * side effects in event handlers, not in render).
 */
function InstanceSizeFormFields({
    isCreate,
    formRef,
}: {
    isCreate: boolean;
    formRef: React.RefObject<DynamicSchemaFormHandle | null>;
}) {
    const { t } = useTranslation(['admin', 'common']);

    return (
        <>
            {/* ── Basic Info ── */}
            <Divider orientation="left" plain style={{ marginTop: 0 }}>
                {t('instanceSizes.section_basic')}
            </Divider>

            <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}>
                <Input placeholder="gpu-workstation" disabled={!isCreate} />
            </Form.Item>
            <Form.Item name="display_name" label={t('common:table.display_name')}>
                <Input placeholder="GPU Workstation (8 cores 32GB)" />
            </Form.Item>
            <Form.Item name="description" label={t('common:table.description')}>
                <Input.TextArea rows={2} />
            </Form.Item>
            <Form.Item
                name="catalog_scope"
                label={t('instanceSizes.catalog_scope')}
                initialValue="unclassified"
                extra={t('instanceSizes.catalog_scope_help')}
                rules={[
                    { required: true, message: t('instanceSizes.catalog_scope_required') },
                ]}
            >
                <Select
                    options={[
                        { label: t('templates.scope_unclassified'), value: 'unclassified' },
                        { label: t('templates.scope_test'), value: 'test' },
                        { label: t('templates.scope_prod'), value: 'prod' },
                        { label: t('templates.scope_all'), value: 'all' },
                    ]}
                />
            </Form.Item>
            <Form.Item name="sort_order" label={t('instanceSizes.sort_order')}>
                <InputNumber style={{ width: '100%' }} />
            </Form.Item>

            {/* ── Resource Configuration ── */}
            <Divider orientation="left" plain>
                {t('instanceSizes.section_resources')}
            </Divider>

            <Form.Item name="cpu_cores" label={t('instanceSizes.cpu')} rules={[{ required: true }]}>
                <UnitInputNumber min={0.5} step={0.5} precision={1} unit={t('instanceSizes.cores')} />
            </Form.Item>

            <Form.Item noStyle shouldUpdate={(prev, cur) => prev.cpu_overcommit_enabled !== cur.cpu_overcommit_enabled}>
                {({ getFieldValue }) => (
                    <Form.Item name="dedicated_cpu" valuePropName="checked">
                        <Checkbox disabled={!!getFieldValue('cpu_overcommit_enabled')}>
                            {t('instanceSizes.dedicated')}
                        </Checkbox>
                    </Form.Item>
                )}
            </Form.Item>

            {/* CPU Overcommit: conditional reveal using shouldUpdate (rendering only, no side effects) */}
            <Form.Item noStyle shouldUpdate={(prev, cur) => prev.dedicated_cpu !== cur.dedicated_cpu}>
                {({ getFieldValue }) => (
                    <Form.Item name="cpu_overcommit_enabled" valuePropName="checked">
                        <Checkbox disabled={!!getFieldValue('dedicated_cpu')}>
                            {t('instanceSizes.enable_cpu_overcommit')}
                        </Checkbox>
                    </Form.Item>
                )}
            </Form.Item>
            <Form.Item
                noStyle
                shouldUpdate={(prev, cur) =>
                    prev.cpu_overcommit_enabled !== cur.cpu_overcommit_enabled ||
                    prev.dedicated_cpu !== cur.dedicated_cpu
                }
            >
                {({ getFieldValue }) =>
                    getFieldValue('cpu_overcommit_enabled') && !getFieldValue('dedicated_cpu') ? (
                        <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
                            <Space style={{ width: '100%' }} direction="vertical">
                                <Form.Item name="cpu_request" label={t('instanceSizes.cpu_request')} style={{ margin: 0 }}>
                                    <UnitInputNumber min={0.5} step={0.5} precision={1} unit={t('instanceSizes.cores')} />
                                </Form.Item>
                                <Text type="secondary" style={{ fontSize: 12 }}>
                                    {t('instanceSizes.overcommit_ratio_hint')}
                                </Text>
                            </Space>
                        </Card>
                    ) : null
                }
            </Form.Item>

            <Form.Item name="memory_gi" label={t('instanceSizes.memory')} rules={[{ required: true }]}>
                <UnitInputNumber min={0.5} step={0.5} precision={1} unit="Gi" />
            </Form.Item>

            {/* Memory Overcommit: conditional reveal */}
            <Form.Item name="memory_overcommit_enabled" valuePropName="checked">
                <Checkbox>{t('instanceSizes.enable_memory_overcommit')}</Checkbox>
            </Form.Item>
            <Form.Item
                noStyle
                shouldUpdate={(prev, cur) => prev.memory_overcommit_enabled !== cur.memory_overcommit_enabled}
            >
                {({ getFieldValue }) =>
                    getFieldValue('memory_overcommit_enabled') ? (
                        <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
                            <Form.Item name="memory_request_gi" label={t('instanceSizes.memory_request')} style={{ margin: 0 }}>
                                <UnitInputNumber min={0.5} step={0.5} precision={1} unit="Gi" />
                            </Form.Item>
                        </Card>
                    ) : null
                }
            </Form.Item>

            <Form.Item name="disk_gb" label={t('instanceSizes.disk')}>
                <UnitInputNumber min={1} unit="GB" />
            </Form.Item>

            <Form.Item name="requires_sriov" label={t('instanceSizes.sriov')} valuePropName="checked">
                <Switch />
            </Form.Item>

            {/* ── Spec Overrides (Schema-driven, ADR-0023 Stage 1) ── */}
            <Divider orientation="left" plain>
                {t('instanceSizes.section_advanced')}
            </Divider>

            {/*
             * DynamicSchemaForm renders KubeVirt spec fields driven by the
             * mask from GET /schemas/instancesize.  This replaces the previous
             * hardcoded Hugepages Select + GPU Form.List + JSON textarea.
             *
             * Data flow:
             *   spec_text (JSON string in Form) ←→ DynamicSchemaForm
             *   onValuesChange → formRef.current?.sync() → spec_text updated
             */}
            <InstanceSizeSpecSection formRef={formRef} disabled={false} />

            <Form.Item name="enabled" label={t('instanceSizes.enabled')} valuePropName="checked" initialValue={true} style={{ marginTop: 16 }}>
                <Switch />
            </Form.Item>
        </>
    );
}

export function AdminInstanceSizesContent() {
    const { t } = useTranslation(['admin', 'common']);
    const sizes = useAdminInstanceSizesController({ t });
    const searchInputRef = useRef<InputRef>(null);
    const catalogScopeOptions = [
        { label: t('templates.scope_unclassified'), value: 'unclassified' },
        { label: t('templates.scope_test'), value: 'test' },
        { label: t('templates.scope_prod'), value: 'prod' },
        { label: t('templates.scope_all'), value: 'all' },
    ];
    const catalogScopeFilters = catalogScopeOptions.map((option) => ({
        text: option.label,
        value: option.value,
    }));

    // Refs for DynamicSchemaForm imperative sync (antd best practice).
    // formRef.current?.sync() is called in onValuesChange to update spec_text.
    const createFormRef = useRef<DynamicSchemaFormHandle>(null);
    const editFormRef = useRef<DynamicSchemaFormHandle>(null);

    const getColumnSearchProps = (dataIndex: keyof InstanceSize): Partial<ColumnsType<InstanceSize>[number]> => ({
        filterDropdown: ({ setSelectedKeys, selectedKeys, confirm, clearFilters }: FilterDropdownProps) => (
            <div style={{ padding: 8 }} onKeyDown={(e) => e.stopPropagation()}>
                <Input
                    ref={searchInputRef}
                    placeholder={`${t('common:button.search')} ${dataIndex}`}
                    value={selectedKeys[0]}
                    onChange={(e) => setSelectedKeys(e.target.value ? [e.target.value] : [])}
                    onPressEnter={() => {
                        confirm();
                        sizes.setSearchText(selectedKeys[0] as string);
                        sizes.setSearchedColumn(dataIndex);
                    }}
                    style={{ marginBottom: 8, display: 'block' }}
                />
                <Space>
                    <Button
                        type="primary"
                        onClick={() => {
                            confirm();
                            sizes.setSearchText(selectedKeys[0] as string);
                            sizes.setSearchedColumn(dataIndex);
                        }}
                        icon={<SearchOutlined />}
                        size="small"
                        style={{ width: 90 }}
                    >
                        {t('common:button.search')}
                    </Button>
                    <Button
                        onClick={() => {
                            clearFilters?.();
                            sizes.setSearchText('');
                            sizes.setSearchedColumn('');
                            confirm();
                        }}
                        size="small"
                        style={{ width: 90 }}
                    >
                        {t('common:button.reset')}
                    </Button>
                </Space>
            </div>
        ),
        filterIcon: (filtered: boolean) => (
            <SearchOutlined style={{ color: filtered ? '#1677ff' : undefined }} />
        ),
        onFilter: (value, record) =>
            (record[dataIndex] ?? '').toString().toLowerCase().includes((value as string).toLowerCase()),
        filterDropdownProps: {
            onOpenChange: (visible) => {
                if (visible) {
                    setTimeout(() => searchInputRef.current?.select(), 100);
                }
            },
        },
    });

    const columns: ColumnsType<InstanceSize> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            ...getColumnSearchProps('name'),
            sorter: (a, b) => a.name.localeCompare(b.name),
            render: (name: string, record: InstanceSize) => (
                <Space>
                    <HddOutlined style={{ color: '#1677ff' }} />
                    <div>
                        {/*
                         * Empty-string display_name should fall back to name.
                         * The API returns "" for unset optional strings, so ?? would
                         * render a blank primary label and make the row look missing.
                         */}
                        <Text strong>
                            {(() => {
                                const displayName = record.display_name || name;
                                return sizes.searchedColumn === 'name'
                                    ? highlightText(displayName, sizes.searchText)
                                    : displayName;
                            })()}
                        </Text>
                        <br />
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {sizes.searchedColumn === 'name' ? highlightText(name, sizes.searchText) : name}
                        </Text>
                    </div>
                </Space>
            ),
        },
        {
            title: t('instanceSizes.cpu'),
            dataIndex: 'cpu_cores',
            key: 'cpu_cores',
            width: 100,
            align: 'center' as const,
            sorter: (a, b) => a.cpu_cores - b.cpu_cores,
            render: (cores: number, record: InstanceSize) => (
                <Space direction="vertical" size={0} style={{ textAlign: 'center' }}>
                    <Text strong>{formatCores(cores)} {t('instanceSizes.cores')}</Text>
                    {record.dedicated_cpu && (
                        <Tag color="orange" style={{ fontSize: 10 }}>
                            <ThunderboltOutlined /> {t('instanceSizes.dedicated')}
                        </Tag>
                    )}
                    {hasCPUOvercommit(record) && (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {t('instanceSizes.request_compact', {
                                value: `${formatCores(record.cpu_request!)} ${t('instanceSizes.cores')}`,
                            })}
                        </Text>
                    )}
                </Space>
            ),
        },
        {
            title: t('instanceSizes.memory'),
            dataIndex: 'memory_gi',
            key: 'memory_gi',
            width: 100,
            align: 'center' as const,
            sorter: (a, b) => a.memory_gi - b.memory_gi,
            render: (gi: number, record: InstanceSize) => (
                <Space direction="vertical" size={0} style={{ textAlign: 'center' }}>
                    <Text strong>{formatMemory(gi)}</Text>
                    {hasMemoryOvercommit(record) && (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {t('instanceSizes.request_compact', {
                                value: formatMemory(record.memory_request_gi!),
                            })}
                        </Text>
                    )}
                </Space>
            ),
        },
        {
            title: t('instanceSizes.catalog_scope'),
            dataIndex: 'catalog_scope',
            key: 'catalog_scope',
            width: 120,
            render: (scope: InstanceSize['catalog_scope']) => (
                <Tag color={catalogScopeColor(scope)}>
                    {catalogScopeLabel(scope, t)}
                </Tag>
            ),
            filters: catalogScopeFilters,
            onFilter: (value, record) => record.catalog_scope === value,
        },
        {
            title: t('instanceSizes.disk'),
            dataIndex: 'disk_gb',
            key: 'disk_gb',
            width: 100,
            align: 'center' as const,
            sorter: (a, b) => (a.disk_gb ?? 0) - (b.disk_gb ?? 0),
            render: (gb: number | undefined) => gb ? <Text>{gb} GB</Text> : <Text type="secondary">—</Text>,
        },
        {
            title: t('instanceSizes.capabilities'),
            key: 'capabilities',
            width: 200,
            render: (_: unknown, record: InstanceSize) => {
                const tags: { label: string; color: string }[] = [];
                const gpuDevices = getGPUDeviceLabels(record);
                if (gpuDevices.length > 0) {
                    tags.push(...gpuDevices.map((device) => ({ label: `GPU ${device}`, color: 'volcano' })));
                } else if (record.requires_gpu) {
                    tags.push({ label: 'GPU', color: 'volcano' });
                }
                if (record.requires_sriov) tags.push({ label: 'SR-IOV', color: 'purple' });
                if (record.requires_hugepages) {
                    tags.push({ label: `Hugepages ${record.hugepages_size ?? ''}`.trim(), color: 'geekblue' });
                }
                if (tags.length === 0) return <Text type="secondary">—</Text>;
                return (
                    <Space size={[0, 4]} wrap>
                        {tags.map((tag) => (
                            <Tag key={tag.label} color={tag.color}>{tag.label}</Tag>
                        ))}
                    </Space>
                );
            },
        },
        {
            title: t('instanceSizes.enabled'),
            dataIndex: 'enabled',
            key: 'enabled',
            width: 90,
            render: (enabled: boolean | undefined) => (
                <Tag color={enabled !== false ? 'green' : 'default'}>
                    {enabled !== false ? t('common:status.active') : t('common:status.disabled')}
                </Tag>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 120,
            render: (_: unknown, record: InstanceSize) => (
                <Space size="small">
                    <Tooltip title={t('common:button.edit')}>
                        <Button
                            type="text"
                            size="small"
                            data-testid={`instance-size-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            onClick={() => sizes.openEditModal(record)}
                        />
                    </Tooltip>
                    <Tooltip title={t('common:button.delete')}>
                        <Button
                            type="text"
                            size="small"
                            danger
                            data-testid={`instance-size-action-delete-${record.id}`}
                            icon={<DeleteOutlined />}
                            onClick={() => sizes.openDeleteModal(record)}
                        />
                    </Tooltip>
                </Space>
            ),
        },
    ];

    return (
        <div>
            {sizes.messageContextHolder}
            <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 24,
            }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('instanceSizes.title')}</Title>
                    <Text type="secondary">{t('instanceSizes.subtitle')}</Text>
                </div>
                <Space>
                    <Input
                        placeholder={t('common:button.search')}
                        prefix={<SearchOutlined />}
                        value={sizes.globalSearch}
                        onChange={(e) => sizes.setGlobalSearch(e.target.value)}
                        allowClear
                        style={{ width: 220 }}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => sizes.refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <Button
                        type="primary"
                        icon={<HddOutlined />}
                        data-testid="instance-size-create-button"
                        onClick={sizes.openCreateModal}
                    >
                        {t('common:button.add')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                {sizes.listError && (
                    <Alert
                        type="error"
                        showIcon
                        style={{ margin: 16, marginBottom: 0 }}
                        message={t('instanceSizes.load_error', 'Failed to load instance sizes')}
                        description={sizes.listError.message || t('common:message.error')}
                        action={
                            <Button size="small" onClick={() => sizes.refetch()}>
                                {t('common:button.refresh')}
                            </Button>
                        }
                    />
                )}
                <div style={{
                    opacity: sizes.isStale ? 0.6 : 1,
                    transition: sizes.isStale ? 'opacity 0.2s 0.1s linear' : 'opacity 0s 0s linear',
                }}>
                    <Table<InstanceSize>
                        columns={columns}
                        dataSource={sizes.filteredItems}
                        rowKey="id"
                        loading={sizes.isLoading}
                        size="middle"
                        pagination={{
                            total: sizes.filteredItems.length,
                            pageSize: 20,
                            showTotal: (total) => t('common:table.total', { total }),
                            showSizeChanger: false,
                        }}
                        locale={{
                            emptyText: (
                                <Empty
                                    description={
                                        sizes.deferredSearch
                                            ? t('common:message.no_data')
                                            : t('instanceSizes.empty')
                                    }
                                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                                />
                            ),
                        }}
                    />
                </div>
            </Card>

            {/* Create Modal */}
            <Modal
                title={t('instanceSizes.create_title')}
                open={sizes.createOpen}
                onOk={() => { void sizes.submitCreate(); }}
                onCancel={sizes.closeCreateModal}
                confirmLoading={sizes.createPending}
                forceRender
                destroyOnHidden={true}
                width={720}
                data-testid="instance-size-create-modal"
            >
                <Form
                    form={sizes.createForm}
                    layout="vertical"
                    onValuesChange={(changedValues) => {
                        handleInstanceSizeFormValuesChange(sizes.createForm, createFormRef, changedValues);
                    }}
                >
                    <InstanceSizeFormFields isCreate={true} formRef={createFormRef} />
                </Form>
            </Modal>

            {/* Edit Modal */}
            <Modal
                title={t('instanceSizes.edit_title')}
                open={sizes.editOpen}
                onOk={() => { void sizes.submitEdit(); }}
                onCancel={sizes.closeEditModal}
                confirmLoading={sizes.updatePending}
                forceRender
                destroyOnHidden={true}
                width={720}
                data-testid="instance-size-edit-modal"
            >
                <Form
                    form={sizes.editForm}
                    layout="vertical"
                    onValuesChange={(changedValues) => {
                        handleInstanceSizeFormValuesChange(sizes.editForm, editFormRef, changedValues);
                    }}
                >
                    <InstanceSizeFormFields isCreate={false} formRef={editFormRef} />
                </Form>
            </Modal>

            {/* Delete Modal */}
            <Modal
                title={t('common:button.delete')}
                open={sizes.deleteOpen}
                onOk={sizes.submitDelete}
                onCancel={sizes.closeDeleteModal}
                confirmLoading={sizes.deletePending}
                okButtonProps={{ danger: true }}
            >
                <Text>
                    {t('common:message.delete_confirm', {
                        name: sizes.deletingItem?.display_name || sizes.deletingItem?.name || '-',
                    })}
                </Text>
            </Modal>
        </div>
    );
}
