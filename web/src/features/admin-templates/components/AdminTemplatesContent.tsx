'use client';

import { useRef } from 'react';
import {
    Alert,
    Button,
    Card,
    Divider,
    Empty,
    Form,
    Input,
    Modal,
    Radio,
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
    FileTextOutlined,
    PlusOutlined,
    ReloadOutlined,
    SearchOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { useAdminTemplatesController } from '../hooks/useAdminTemplatesController';
import { OS_COLOR_MAP, type Template } from '../types';
import { getTemplateRequestFlowStatus } from '../requestFlow';

const { Title, Text } = Typography;

function ExperimentalSourceGate({
    title,
    description,
    buttonLabel,
    onEnable,
}: {
    title: string;
    description: string;
    buttonLabel: string;
    onEnable: () => void;
}) {
    return (
        <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message={title}
            description={description}
            action={
                <Button size="small" onClick={onEnable}>
                    {buttonLabel}
                </Button>
            }
        />
    );
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

export function AdminTemplatesContent() {
    const { t } = useTranslation(['admin', 'common', 'error']);
    const templates = useAdminTemplatesController({ t });
    const searchInputRef = useRef<InputRef>(null);
    const catalogScopeOptions = [
        { label: t('templates.scope_unclassified'), value: 'unclassified' },
        { label: t('templates.scope_test'), value: 'test' },
        { label: t('templates.scope_prod'), value: 'prod' },
        { label: t('templates.scope_all'), value: 'all' },
    ];

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
            title: t('templates.catalog_scope'),
            dataIndex: 'catalog_scope',
            key: 'catalog_scope',
            width: 120,
            render: (scope: string | undefined) => {
                const normalized = (scope ?? 'unclassified').toLowerCase();
                const color = normalized === 'prod' ? 'red' : normalized === 'all' ? 'blue' : normalized === 'test' ? 'gold' : 'default';
                return <Tag color={color}>{t(`templates.scope_${normalized}`, { defaultValue: normalized })}</Tag>;
            },
        },
        {
            title: t('templates.request_flow'),
            key: 'request_flow',
            width: 240,
            render: (_: unknown, record: Template) => {
                const status = getTemplateRequestFlowStatus(record);
                switch (status) {
                    case 'self_service':
                        return <Tag color="green">{t('templates.request_flow_self_service')}</Tag>;
                    case 'admin_only_source':
                        return (
                            <div>
                                <Tag color="orange">{t('templates.request_flow_admin_only')}</Tag>
                                <div>
                                    <Text type="secondary" style={{ fontSize: 12 }}>
                                        {t('templates.request_flow_reason_admin_only')}
                                    </Text>
                                </div>
                            </div>
                        );
                    case 'hidden_unclassified':
                        return (
                            <div>
                                <Tag color="gold">{t('templates.request_flow_hidden')}</Tag>
                                <div>
                                    <Text type="secondary" style={{ fontSize: 12 }}>
                                        {t('templates.request_flow_reason_hidden')}
                                    </Text>
                                </div>
                            </div>
                        );
                    case 'disabled':
                        return (
                            <div>
                                <Tag>{t('templates.request_flow_disabled')}</Tag>
                                <div>
                                    <Text type="secondary" style={{ fontSize: 12 }}>
                                        {t('templates.request_flow_reason_disabled')}
                                    </Text>
                                </div>
                            </div>
                        );
                    default:
                        return (
                            <div>
                                <Tag color="red">{t('templates.request_flow_unavailable')}</Tag>
                                <div>
                                    <Text type="secondary" style={{ fontSize: 12 }}>
                                        {t('templates.request_flow_reason_unsupported')}
                                    </Text>
                                </div>
                            </div>
                        );
                }
            },
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
                            data-testid={`template-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            onClick={() => templates.openEditModal(record)}
                        />
                    </Tooltip>
                    <Tooltip title={t('common:button.delete')}>
                        <Button
                            type="text"
                            size="small"
                            danger
                            data-testid={`template-action-delete-${record.id}`}
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
                        data-testid="template-create-button"
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

            {/* ── Create Modal (master-flow Step 3) ── */}
            <Modal
                title={t('common:button.add')}
                open={templates.createOpen}
                onOk={() => { void templates.submitCreate(); }}
                onCancel={templates.closeCreateModal}
                confirmLoading={templates.createPending}
                destroyOnHidden={true}
                width={640}
                data-testid="template-create-modal"
            >
                <Form form={templates.createForm} layout="vertical" preserve={false}>
                    <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}>
                        <Input placeholder="centos7-standard" />
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
                        name="catalog_scope"
                        label={t('templates.catalog_scope')}
                        initialValue="unclassified"
                        dependencies={['source_type']}
                        extra={t('templates.catalog_scope_help')}
                        rules={[
                            { required: true, message: t('templates.catalog_scope_required') },
                            ({ getFieldValue }) => ({
                                validator: async (_, value: string | undefined) => {
                                    if (getFieldValue('source_type') === 'containerdisk' && (value === 'prod' || value === 'all')) {
                                        throw new Error(t('templates.containerdisk_scope_invalid'));
                                    }
                                },
                            }),
                        ]}
                    >
                        <Select options={catalogScopeOptions} />
                    </Form.Item>

                    {/* Boot Source — three-mode taxonomy from the current backend canonical model */}
                    <Divider orientation="left" plain>{t('templates.image_source')}</Divider>
                    {!templates.createExperimentalSourcesEnabled ? (
                        <ExperimentalSourceGate
                            title={t('templates.experimental_source_title')}
                            description={t('templates.experimental_source_description')}
                            buttonLabel={t('templates.experimental_source_enable')}
                            onEnable={templates.enableCreateExperimentalSources}
                        />
                    ) : null}
                    <Form.Item name="source_type" label={t('templates.source_type')} initialValue="cdi_image_import">
                        <Radio.Group>
                            <Radio value="cdi_image_import">{t('templates.source_cdi_import')}</Radio>
                            <Radio value="cdi_pvc_clone">{t('templates.source_cdi_clone')}</Radio>
                            {templates.createExperimentalSourcesEnabled ? (
                                <Radio value="containerdisk">{t('templates.source_containerdisk')}</Radio>
                            ) : null}
                        </Radio.Group>
                    </Form.Item>
                    <Form.Item
                        noStyle
                        shouldUpdate={(prev, cur) =>
                            prev.source_type !== cur.source_type || prev.catalog_scope !== cur.catalog_scope
                        }
                    >
                        {({ getFieldValue }) => {
                            const sourceType = getFieldValue('source_type');
                            const scope = getFieldValue('catalog_scope');
                            if (sourceType !== 'containerdisk') {
                                return null;
                            }
                            const invalidScope = scope === 'prod' || scope === 'all';
                            return (
                                <Alert
                                    type={invalidScope ? 'error' : 'warning'}
                                    showIcon
                                    style={{ marginBottom: 16 }}
                                    message={invalidScope ? t('templates.containerdisk_scope_invalid') : t('templates.containerdisk_scope_warning')}
                                    description={t('templates.request_flow_reason_admin_only')}
                                />
                            );
                        }}
                    </Form.Item>
                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.source_type !== cur.source_type}>
                        {({ getFieldValue }) =>
                            getFieldValue('source_type') === 'cdi_pvc_clone' ? (
                                <>
                                    <Form.Item
                                        name="pvc_namespace"
                                        label={t('templates.pvc_namespace')}
                                        rules={[{ required: true, message: t('templates.pvc_namespace_required') }]}
                                        extra={t('templates.pvc_namespace_help')}
                                    >
                                        <Input placeholder="default" />
                                    </Form.Item>
                                    <Form.Item
                                        name="pvc_name"
                                        label={t('templates.pvc_name')}
                                        rules={[{ required: true, message: t('templates.pvc_name_required') }]}
                                        extra={t('templates.pvc_name_help')}
                                    >
                                        <Input placeholder="centos7-base-disk" />
                                    </Form.Item>
                                </>
                            ) : (
                                <Form.Item name="image_url" label={t('templates.image_url')} rules={[{ required: true, message: t('templates.image_url_required') }]}>
                                    <Input placeholder="docker.io/kubevirt/centos:7" />
                                </Form.Item>
                            )
                        }
                    </Form.Item>

                    {/* cloud-init config — master-flow Step 3: YAML text, NOT JSON spec */}
                    <Divider orientation="left" plain>{t('templates.cloud_init')}</Divider>
                    <Form.Item
                        name="cloud_init"
                        extra={t('templates.cloud_init_help', 'YAML cloud-init configuration. Provides one-time password and initial OS setup.')}
                    >
                        <Input.TextArea
                            rows={8}
                            style={{ fontFamily: 'monospace', fontSize: 13 }}
                            placeholder={'#cloud-config\nusers:\n  - name: admin\n    sudo: ALL=(ALL) NOPASSWD:ALL\nchpasswd:\n  expire: true\n  users:\n    - name: admin\n      password: changeme123'}
                        />
                    </Form.Item>

                    <Form.Item name="enabled" label={t('templates.enabled')} valuePropName="checked" initialValue={true}>
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            {/* ── Edit Modal (master-flow Step 3) ── */}
            <Modal
                title={t('common:button.edit')}
                open={templates.editOpen}
                onOk={() => { void templates.submitEdit(); }}
                onCancel={templates.closeEditModal}
                confirmLoading={templates.updatePending}
                destroyOnHidden={true}
                width={640}
                data-testid="template-edit-modal"
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
                        name="catalog_scope"
                        label={t('templates.catalog_scope')}
                        dependencies={['source_type']}
                        extra={t('templates.catalog_scope_help')}
                        rules={[
                            { required: true, message: t('templates.catalog_scope_required') },
                            ({ getFieldValue }) => ({
                                validator: async (_, value: string | undefined) => {
                                    if (getFieldValue('source_type') === 'containerdisk' && (value === 'prod' || value === 'all')) {
                                        throw new Error(t('templates.containerdisk_scope_invalid'));
                                    }
                                },
                            }),
                        ]}
                    >
                        <Select options={catalogScopeOptions} />
                    </Form.Item>

                    {/* Image Source toggle */}
                    <Divider orientation="left" plain>{t('templates.image_source')}</Divider>
                    {!templates.editExperimentalSourcesEnabled ? (
                        <ExperimentalSourceGate
                            title={t('templates.experimental_source_title')}
                            description={t('templates.experimental_source_description')}
                            buttonLabel={t('templates.experimental_source_enable')}
                            onEnable={templates.enableEditExperimentalSources}
                        />
                    ) : null}
                    <Form.Item name="source_type" label={t('templates.source_type')}>
                        <Radio.Group>
                            <Radio value="cdi_image_import">{t('templates.source_cdi_import')}</Radio>
                            <Radio value="cdi_pvc_clone">{t('templates.source_cdi_clone')}</Radio>
                            {templates.editExperimentalSourcesEnabled ? (
                                <Radio value="containerdisk">{t('templates.source_containerdisk')}</Radio>
                            ) : null}
                        </Radio.Group>
                    </Form.Item>
                    <Form.Item
                        noStyle
                        shouldUpdate={(prev, cur) =>
                            prev.source_type !== cur.source_type || prev.catalog_scope !== cur.catalog_scope
                        }
                    >
                        {({ getFieldValue }) => {
                            const sourceType = getFieldValue('source_type');
                            const scope = getFieldValue('catalog_scope');
                            if (sourceType !== 'containerdisk') {
                                return null;
                            }
                            const invalidScope = scope === 'prod' || scope === 'all';
                            return (
                                <Alert
                                    type={invalidScope ? 'error' : 'warning'}
                                    showIcon
                                    style={{ marginBottom: 16 }}
                                    message={invalidScope ? t('templates.containerdisk_scope_invalid') : t('templates.containerdisk_scope_warning')}
                                    description={t('templates.request_flow_reason_admin_only')}
                                />
                            );
                        }}
                    </Form.Item>
                    <Form.Item noStyle shouldUpdate={(prev, cur) => prev.source_type !== cur.source_type}>
                        {({ getFieldValue }) =>
                            getFieldValue('source_type') === 'cdi_pvc_clone' ? (
                                <>
                                    <Form.Item
                                        name="pvc_namespace"
                                        label={t('templates.pvc_namespace')}
                                        rules={[{ required: true, message: t('templates.pvc_namespace_required') }]}
                                        extra={t('templates.pvc_namespace_help')}
                                    >
                                        <Input placeholder="default" />
                                    </Form.Item>
                                    <Form.Item
                                        name="pvc_name"
                                        label={t('templates.pvc_name')}
                                        rules={[{ required: true, message: t('templates.pvc_name_required') }]}
                                        extra={t('templates.pvc_name_help')}
                                    >
                                        <Input placeholder="centos7-base-disk" />
                                    </Form.Item>
                                </>
                            ) : (
                                <Form.Item name="image_url" label={t('templates.image_url')}>
                                    <Input placeholder="docker.io/kubevirt/centos:7" />
                                </Form.Item>
                            )
                        }
                    </Form.Item>

                    {/* cloud-init YAML editor */}
                    <Divider orientation="left" plain>{t('templates.cloud_init')}</Divider>
                    <Form.Item
                        name="cloud_init"
                        extra={t('templates.cloud_init_help', 'YAML cloud-init configuration. Provides one-time password and initial OS setup.')}
                    >
                        <Input.TextArea
                            rows={8}
                            style={{ fontFamily: 'monospace', fontSize: 13 }}
                            placeholder={'#cloud-config\nusers:\n  - name: admin'}
                        />
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
