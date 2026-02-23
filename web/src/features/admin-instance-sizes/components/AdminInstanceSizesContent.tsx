'use client';

import { useRef } from 'react';
import {
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
    Switch,
    Table,
    Tag,
    Tooltip,
    Typography,
} from 'antd';
import type { InputRef } from 'antd';
import type { ColumnsType, FilterDropdownProps } from 'antd/es/table/interface';
import {
    DeleteOutlined,
    EditOutlined,
    HddOutlined,
    MinusCircleOutlined,
    PlusOutlined,
    ReloadOutlined,
    SearchOutlined,
    ThunderboltOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { useAdminInstanceSizesController } from '../hooks/useAdminInstanceSizesController';
import { formatMemory, type InstanceSize } from '../types';

const { Title, Text } = Typography;

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
 * Shared form fields for InstanceSize create/edit modals.
 * Uses Ant Design shouldUpdate pattern for conditional overcommit sections
 * and Form.List for GPU devices (per master-flow Stage 3 Step 4 design).
 */
function InstanceSizeFormFields({ isCreate }: { isCreate: boolean }) {
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
            <Form.Item name="sort_order" label={t('instanceSizes.sort_order')}>
                <InputNumber style={{ width: '100%' }} />
            </Form.Item>

            {/* ── Resource Configuration ── */}
            <Divider orientation="left" plain>
                {t('instanceSizes.section_resources')}
            </Divider>

            <Form.Item name="cpu_cores" label={t('instanceSizes.cpu')} rules={[{ required: true }]}>
                <InputNumber min={1} style={{ width: '100%' }} addonAfter={t('instanceSizes.cores')} />
            </Form.Item>

            {/* CPU Overcommit: conditional reveal */}
            <Form.Item name="cpu_overcommit_enabled" valuePropName="checked">
                <Checkbox>{t('instanceSizes.enable_cpu_overcommit')}</Checkbox>
            </Form.Item>
            <Form.Item
                noStyle
                shouldUpdate={(prev, cur) => prev.cpu_overcommit_enabled !== cur.cpu_overcommit_enabled}
            >
                {({ getFieldValue }) =>
                    getFieldValue('cpu_overcommit_enabled') ? (
                        <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
                            <Space style={{ width: '100%' }} direction="vertical">
                                <Form.Item name="cpu_request" label={t('instanceSizes.cpu_request')} style={{ margin: 0 }}>
                                    <InputNumber min={1} style={{ width: '100%' }} addonAfter={t('instanceSizes.cores')} />
                                </Form.Item>
                                <Text type="secondary" style={{ fontSize: 12 }}>
                                    {t('instanceSizes.overcommit_ratio_hint')}
                                </Text>
                            </Space>
                        </Card>
                    ) : null
                }
            </Form.Item>

            <Form.Item name="memory_mb" label={t('instanceSizes.memory')} rules={[{ required: true }]}>
                <InputNumber min={1} style={{ width: '100%' }} addonAfter="MB" />
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
                            <Form.Item name="memory_request_mb" label={t('instanceSizes.memory_request')} style={{ margin: 0 }}>
                                <InputNumber min={1} style={{ width: '100%' }} addonAfter="MB" />
                            </Form.Item>
                        </Card>
                    ) : null
                }
            </Form.Item>

            <Form.Item name="disk_gb" label={t('instanceSizes.disk')}>
                <InputNumber min={1} style={{ width: '100%' }} addonAfter="GB" />
            </Form.Item>

            {/* ── Advanced Settings ── */}
            <Divider orientation="left" plain>
                {t('instanceSizes.section_advanced')}
            </Divider>

            {/* Hugepages: Single Select replacing Switch + text input */}
            <Form.Item name="hugepages_setting" label={t('instanceSizes.hugepages')}>
                <Select
                    options={[
                        { value: 'none', label: 'None' },
                        { value: '2Mi', label: '2Mi' },
                        { value: '1Gi', label: '1Gi' },
                    ]}
                    placeholder={t('instanceSizes.hugepages_placeholder')}
                />
            </Form.Item>

            <Form.Item name="dedicated_cpu" label={t('instanceSizes.dedicated')} valuePropName="checked">
                <Switch />
            </Form.Item>

            <Form.Item name="requires_sriov" label={t('instanceSizes.sriov')} valuePropName="checked">
                <Switch />
            </Form.Item>

            {/* GPU devices: dynamic Form.List */}
            <Form.Item label={t('instanceSizes.gpu_devices')}>
                <Form.List name="gpu_devices">
                    {(fields, { add, remove }) => (
                        <>
                            {fields.map((field) => (
                                <Space key={field.key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                                    <Form.Item
                                        {...field}
                                        name={[field.name, 'name']}
                                        rules={[{ required: true, message: t('instanceSizes.gpu_name_required') }]}
                                        style={{ margin: 0 }}
                                    >
                                        <Input placeholder="gpu1" style={{ width: 120 }} />
                                    </Form.Item>
                                    <Form.Item
                                        {...field}
                                        name={[field.name, 'deviceName']}
                                        rules={[{ required: true, message: t('instanceSizes.gpu_device_required') }]}
                                        style={{ margin: 0 }}
                                    >
                                        <Input placeholder="nvidia.com/GA102GL_A10" style={{ width: 280 }} />
                                    </Form.Item>
                                    <MinusCircleOutlined
                                        onClick={() => remove(field.name)}
                                        style={{ color: '#ff4d4f' }}
                                    />
                                </Space>
                            ))}
                            <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                                {t('instanceSizes.add_gpu')}
                            </Button>
                        </>
                    )}
                </Form.List>
            </Form.Item>

            {/* spec_overrides JSON (for advanced/escape-hatch usage) */}
            <Form.Item
                name="spec_overrides_text"
                label={t('instanceSizes.spec_overrides')}
                extra={t('instanceSizes.spec_overrides_help')}
            >
                <Input.TextArea rows={6} style={{ fontFamily: 'monospace', fontSize: 13 }} />
            </Form.Item>

            <Form.Item name="enabled" label={t('instanceSizes.enabled')} valuePropName="checked" initialValue={true}>
                <Switch />
            </Form.Item>
        </>
    );
}

export function AdminInstanceSizesContent() {
    const { t } = useTranslation(['admin', 'common']);
    const sizes = useAdminInstanceSizesController({ t });
    const searchInputRef = useRef<InputRef>(null);

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
                        <Text strong>
                            {sizes.searchedColumn === 'name'
                                ? highlightText(record.display_name ?? name, sizes.searchText)
                                : (record.display_name ?? name)}
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
                    <Text strong>{cores} {t('instanceSizes.cores')}</Text>
                    {record.dedicated_cpu && (
                        <Tag color="orange" style={{ fontSize: 10 }}>
                            <ThunderboltOutlined /> {t('instanceSizes.dedicated')}
                        </Tag>
                    )}
                </Space>
            ),
        },
        {
            title: t('instanceSizes.memory'),
            dataIndex: 'memory_mb',
            key: 'memory_mb',
            width: 100,
            align: 'center' as const,
            sorter: (a, b) => a.memory_mb - b.memory_mb,
            render: (mb: number) => <Text strong>{formatMemory(mb)}</Text>,
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
                if (record.requires_gpu) tags.push({ label: 'GPU', color: 'volcano' });
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
                        icon={<PlusOutlined />}
                        data-testid="instance-size-create-button"
                        onClick={sizes.openCreateModal}
                    >
                        {t('common:button.add')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
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
                destroyOnHidden={true}
                width={680}
                data-testid="instance-size-create-modal"
            >
                    <Form form={sizes.createForm} layout="vertical" preserve={false}>
                        <InstanceSizeFormFields isCreate={true} />
                    </Form>
            </Modal>

            {/* Edit Modal */}
            <Modal
                title={t('instanceSizes.edit_title')}
                open={sizes.editOpen}
                onOk={() => { void sizes.submitEdit(); }}
                onCancel={sizes.closeEditModal}
                confirmLoading={sizes.updatePending}
                destroyOnHidden={true}
                width={680}
                data-testid="instance-size-edit-modal"
            >
                    <Form form={sizes.editForm} layout="vertical" preserve={false}>
                        <InstanceSizeFormFields isCreate={false} />
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
                        name: sizes.deletingItem?.display_name ?? sizes.deletingItem?.name ?? '-',
                    })}
                </Text>
            </Modal>
        </div>
    );
}
