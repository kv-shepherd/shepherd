'use client';

import { useRef } from 'react';
import {
    Button,
    Card,
    Empty,
    Form,
    Input,
    InputNumber,
    Modal,
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
                            data-testid={`admin-instance-size-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            onClick={() => sizes.openEditModal(record)}
                        />
                    </Tooltip>
                    <Tooltip title={t('common:button.delete')}>
                        <Button
                            type="text"
                            size="small"
                            danger
                            data-testid={`admin-instance-size-action-delete-${record.id}`}
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
                        data-testid="admin-instance-size-create-button"
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

            <Modal
                title={t('common:button.add')}
                open={sizes.createOpen}
                onOk={() => { void sizes.submitCreate(); }}
                onCancel={sizes.closeCreateModal}
                confirmLoading={sizes.createPending}
                destroyOnHidden={true}
            >
                <Form form={sizes.createForm} layout="vertical" preserve={false}>
                    <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="cpu_cores" label={t('instanceSizes.cpu')} rules={[{ required: true }]}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="memory_mb" label={t('instanceSizes.memory')} rules={[{ required: true }]}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="disk_gb" label={t('instanceSizes.disk')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="cpu_request" label={t('instanceSizes.cpu_request')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="memory_request_mb" label={t('instanceSizes.memory_request')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="sort_order" label={t('instanceSizes.sort_order')}>
                        <InputNumber style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="description" label={t('common:table.description')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item name="requires_gpu" label={t('instanceSizes.gpu')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item name="requires_sriov" label={t('instanceSizes.sriov')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item name="requires_hugepages" label={t('instanceSizes.hugepages')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item name="hugepages_size" label={t('instanceSizes.hugepages_size')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="dedicated_cpu" label={t('instanceSizes.dedicated')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item
                        name="spec_overrides_text"
                        label={t('instanceSizes.spec_overrides')}
                        extra={t('instanceSizes.spec_overrides_help')}
                    >
                        <Input.TextArea rows={8} style={{ fontFamily: 'monospace' }} />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('instanceSizes.enabled')} valuePropName="checked" initialValue={true}>
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('common:button.edit')}
                open={sizes.editOpen}
                onOk={() => { void sizes.submitEdit(); }}
                onCancel={sizes.closeEditModal}
                confirmLoading={sizes.updatePending}
                destroyOnHidden={true}
            >
                <Form form={sizes.editForm} layout="vertical" preserve={false}>
                    <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="cpu_cores" label={t('instanceSizes.cpu')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="memory_mb" label={t('instanceSizes.memory')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="disk_gb" label={t('instanceSizes.disk')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="cpu_request" label={t('instanceSizes.cpu_request')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="memory_request_mb" label={t('instanceSizes.memory_request')}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="sort_order" label={t('instanceSizes.sort_order')}>
                        <InputNumber style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="description" label={t('common:table.description')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item name="requires_gpu" label={t('instanceSizes.gpu')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item name="requires_sriov" label={t('instanceSizes.sriov')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item name="requires_hugepages" label={t('instanceSizes.hugepages')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item name="hugepages_size" label={t('instanceSizes.hugepages_size')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="dedicated_cpu" label={t('instanceSizes.dedicated')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item
                        name="spec_overrides_text"
                        label={t('instanceSizes.spec_overrides')}
                        extra={t('instanceSizes.spec_overrides_help')}
                    >
                        <Input.TextArea rows={8} style={{ fontFamily: 'monospace' }} />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('instanceSizes.enabled')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

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
