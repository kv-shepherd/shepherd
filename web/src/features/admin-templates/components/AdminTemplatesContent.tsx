'use client';

import { useRef } from 'react';
import {
    Button,
    Card,
    Empty,
    Form,
    Input,
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
    FileTextOutlined,
    PlusOutlined,
    ReloadOutlined,
    SearchOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { useAdminTemplatesController } from '../hooks/useAdminTemplatesController';
import { OS_COLOR_MAP, type Template } from '../types';

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

export function AdminTemplatesContent() {
    const { t } = useTranslation(['admin', 'common']);
    const templates = useAdminTemplatesController({ t });
    const searchInputRef = useRef<InputRef>(null);

    const getColumnSearchProps = (dataIndex: keyof Template): Partial<ColumnsType<Template>[number]> => ({
        filterDropdown: ({ setSelectedKeys, selectedKeys, confirm, clearFilters }: FilterDropdownProps) => (
            <div style={{ padding: 8 }} onKeyDown={(e) => e.stopPropagation()}>
                <Input
                    ref={searchInputRef}
                    placeholder={`${t('common:button.search')} ${dataIndex}`}
                    value={selectedKeys[0]}
                    onChange={(e) => setSelectedKeys(e.target.value ? [e.target.value] : [])}
                    onPressEnter={() => {
                        confirm();
                        templates.setSearchText(selectedKeys[0] as string);
                        templates.setSearchedColumn(dataIndex);
                    }}
                    style={{ marginBottom: 8, display: 'block' }}
                />
                <Space>
                    <Button
                        type="primary"
                        onClick={() => {
                            confirm();
                            templates.setSearchText(selectedKeys[0] as string);
                            templates.setSearchedColumn(dataIndex);
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
                            templates.setSearchText('');
                            templates.setSearchedColumn('');
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

    const columns: ColumnsType<Template> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            ...getColumnSearchProps('name'),
            sorter: (a, b) => a.name.localeCompare(b.name),
            render: (name: string, record: Template) => (
                <Space>
                    <FileTextOutlined style={{ color: '#1677ff' }} />
                    <div>
                        <Text strong>
                            {templates.searchedColumn === 'name'
                                ? highlightText(record.display_name ?? name, templates.searchText)
                                : (record.display_name ?? name)}
                        </Text>
                        <br />
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {templates.searchedColumn === 'name' ? highlightText(name, templates.searchText) : name}
                        </Text>
                    </div>
                </Space>
            ),
        },
        {
            title: t('templates.os_family'),
            dataIndex: 'os_family',
            key: 'os_family',
            width: 120,
            filters: templates.osFamilyFilters,
            onFilter: (value, record) => record.os_family === value,
            render: (family: string | undefined) => {
                if (!family) {
                    return <Text type="secondary">—</Text>;
                }
                const color = OS_COLOR_MAP[family.toLowerCase()] ?? 'default';
                return <Tag color={color}>{family}</Tag>;
            },
        },
        {
            title: t('templates.os_version'),
            dataIndex: 'os_version',
            key: 'os_version',
            width: 120,
            render: (version: string | undefined) => version ? <Tag>{version}</Tag> : '—',
        },
        {
            title: t('templates.version'),
            dataIndex: 'version',
            key: 'version',
            width: 90,
            align: 'center' as const,
            sorter: (a, b) => a.version - b.version,
            render: (version: number) => (
                <Tag color="processing">v{version}</Tag>
            ),
        },
        {
            title: t('templates.enabled'),
            dataIndex: 'enabled',
            key: 'enabled',
            width: 90,
            filters: [
                { text: t('common:status.active'), value: true },
                { text: t('common:status.disabled'), value: false },
            ],
            onFilter: (value, record) => (record.enabled !== false) === value,
            render: (enabled: boolean | undefined) => (
                <Tag color={enabled !== false ? 'green' : 'default'}>
                    {enabled !== false ? t('common:status.active') : t('common:status.disabled')}
                </Tag>
            ),
        },
        {
            title: t('common:table.description'),
            dataIndex: 'description',
            key: 'description',
            ellipsis: true,
            render: (desc: string | undefined) => (
                <Text type="secondary">{desc || '—'}</Text>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 120,
            render: (_: unknown, record: Template) => (
                <Space size="small">
                    <Tooltip title={t('common:button.edit')}>
                        <Button
                            type="text"
                            size="small"
                            data-testid={`admin-template-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            onClick={() => templates.openEditModal(record)}
                        />
                    </Tooltip>
                    <Tooltip title={t('common:button.delete')}>
                        <Button
                            type="text"
                            size="small"
                            danger
                            data-testid={`admin-template-action-delete-${record.id}`}
                            icon={<DeleteOutlined />}
                            onClick={() => templates.openDeleteModal(record)}
                        />
                    </Tooltip>
                </Space>
            ),
        },
    ];

    return (
        <div>
            {templates.messageContextHolder}
            <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 24,
            }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('templates.title')}</Title>
                    <Text type="secondary">{t('templates.subtitle')}</Text>
                </div>
                <Space>
                    <Input
                        placeholder={t('common:button.search')}
                        prefix={<SearchOutlined />}
                        value={templates.globalSearch}
                        onChange={(e) => templates.setGlobalSearch(e.target.value)}
                        allowClear
                        style={{ width: 220 }}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => templates.refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        data-testid="admin-template-create-button"
                        onClick={templates.openCreateModal}
                    >
                        {t('common:button.add')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <div style={{
                    opacity: templates.isStale ? 0.6 : 1,
                    transition: templates.isStale ? 'opacity 0.2s 0.1s linear' : 'opacity 0s 0s linear',
                }}>
                    <Table<Template>
                        columns={columns}
                        dataSource={templates.filteredItems}
                        rowKey="id"
                        loading={templates.isLoading}
                        pagination={{
                            current: templates.page,
                            total: templates.data?.pagination?.total ?? templates.filteredItems.length,
                            pageSize: 20,
                            onChange: templates.setPage,
                            showTotal: (total) => t('common:table.total', { total }),
                            showSizeChanger: false,
                        }}
                        size="middle"
                        locale={{
                            emptyText: (
                                <Empty
                                    description={
                                        templates.deferredSearch
                                            ? t('common:message.no_data')
                                            : t('templates.empty')
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
                open={templates.createOpen}
                onOk={() => { void templates.submitCreate(); }}
                onCancel={templates.closeCreateModal}
                confirmLoading={templates.createPending}
                destroyOnHidden={true}
            >
                <Form form={templates.createForm} layout="vertical" preserve={false}>
                    <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}> 
                        <Input />
                    </Form.Item>
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="os_family" label={t('templates.os_family')}>
                        <Input placeholder={t('templates.os_family_placeholder')} />
                    </Form.Item>
                    <Form.Item name="os_version" label={t('templates.os_version')}>
                        <Input placeholder={t('templates.os_version_placeholder')} />
                    </Form.Item>
                    <Form.Item name="description" label={t('common:table.description')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item
                        name="spec_text"
                        label={t('templates.spec')}
                        extra={t('templates.spec_help')}
                    >
                        <Input.TextArea rows={10} style={{ fontFamily: 'monospace' }} />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('templates.enabled')} valuePropName="checked" initialValue={true}>
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('common:button.edit')}
                open={templates.editOpen}
                onOk={() => { void templates.submitEdit(); }}
                onCancel={templates.closeEditModal}
                confirmLoading={templates.updatePending}
                destroyOnHidden={true}
            >
                <Form form={templates.editForm} layout="vertical" preserve={false}>
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="os_family" label={t('templates.os_family')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="os_version" label={t('templates.os_version')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="description" label={t('common:table.description')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item
                        name="spec_text"
                        label={t('templates.spec')}
                        extra={t('templates.spec_help')}
                    >
                        <Input.TextArea rows={10} style={{ fontFamily: 'monospace' }} />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('templates.enabled')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('common:button.delete')}
                open={templates.deleteOpen}
                onOk={templates.submitDelete}
                onCancel={templates.closeDeleteModal}
                confirmLoading={templates.deletePending}
                okButtonProps={{ danger: true }}
            >
                <Text>
                    {t('common:message.delete_confirm', {
                        name: templates.deletingTemplate?.display_name ?? templates.deletingTemplate?.name ?? '-',
                    })}
                </Text>
            </Modal>
        </div>
    );
}
