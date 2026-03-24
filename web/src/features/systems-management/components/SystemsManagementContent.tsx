'use client';

import React, { useState } from 'react';
import {
    Button,
    Form,
    Input,
    Modal,
    Space,
    Table,
    Typography,
    Upload,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    AppstoreOutlined,
    EditOutlined,
    DeleteOutlined,
    ExclamationCircleOutlined,
    EyeOutlined,
    PlusOutlined,
    ReloadOutlined,
    TeamOutlined,
    UploadOutlined,
    FileTextOutlined,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeSanitize from 'rehype-sanitize';
import { useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { PermissionGuard } from '@/components/auth/PermissionGuard';
import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SystemsOverviewGlyph } from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import {
    buildDashboardSetupResumeHref,
    resolveNextSetupAction,
} from '@/features/setup-guide/flow';
import { SetupGuideCard } from '@/features/setup-guide/components/SetupGuideCard';
import { useAutoOpenIntent } from '@/features/setup-guide/hooks/useAutoOpenIntent';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { useSystemsManagementController } from '../hooks/useSystemsManagementController';
import { RFC1035_PATTERN, type System } from '../types';
import { SystemMembersModal } from './SystemMembersModal';

const { Text, Paragraph } = Typography;

export function SystemsManagementContent() {
    const { t } = useTranslation('common');
    const router = useRouter();
    const setupGuide = useSetupGuide();
    const systems = useSystemsManagementController({
        t,
        onCreateSuccess: (_system, context) => {
            if (!context.isFirstSystem) {
                return false;
            }
            const nextAction = resolveNextSetupAction(setupGuide, 'system');
            if (!nextAction) {
                return false;
            }
            router.push(buildDashboardSetupResumeHref(nextAction));
            return true;
        },
    });

    const [createPreviewMode, setCreatePreviewMode] = useState(false);
    const [editPreviewMode, setEditPreviewMode] = useState(false);
    const [detailOpen, setDetailOpen] = useState(false);
    const [detailSystem, setDetailSystem] = useState<System | null>(null);

    useAutoOpenIntent('create-system', () => {
        systems.openCreateModal();
    });

    const columns: ColumnsType<System> = [
        {
            title: t('table.name'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string) => (
                <Space>
                    <AppstoreOutlined style={{ color: '#1677ff' }} />
                    <Text strong>{name}</Text>
                </Space>
            ),
        },
        {
            title: t('table.description'),
            dataIndex: 'description',
            key: 'description',
            ellipsis: true,
            render: (desc: string) => <Text type="secondary">{desc || '—'}</Text>,
        },
        {
            title: t('table.created_by'),
            dataIndex: 'created_by',
            key: 'created_by',
            width: 140,
        },
        {
            title: t('table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (date: string) => (
                <Text type="secondary"><LocalDateTimeText value={date} /></Text>
            ),
        },
        {
            title: t('table.actions'),
            key: 'actions',
            width: 200,
            render: (_, record) => (
                <Space>
                    <Button
                        type="link"
                        size="small"
                        data-testid={`system-action-detail-${record.id}`}
                        icon={<EyeOutlined />}
                        onClick={() => {
                            setDetailSystem(record);
                            setDetailOpen(true);
                        }}
                    >
                        {t('button.detail', 'Details')}
                    </Button>
                    <PermissionGuard permission="rbac:manage">
                        <Button
                            type="link"
                            size="small"
                            data-testid={`system-action-members-${record.id}`}
                            icon={<TeamOutlined />}
                            onClick={() => systems.openMembersModal(record)}
                        >
                            {t('button.manage_members')}
                        </Button>
                    </PermissionGuard>
                    <PermissionGuard permission="system:write">
                        <Button
                            type="link"
                            size="small"
                            data-testid={`system-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            loading={systems.updatePending && systems.editingSystem?.id === record.id}
                            onClick={() => systems.openEditModal(record)}
                        >
                            {t('button.edit')}
                        </Button>
                    </PermissionGuard>
                    <PermissionGuard permission="system:delete">
                        <Button
                            type="link"
                            size="small"
                            data-testid={`system-action-delete-${record.id}`}
                            danger
                            icon={<DeleteOutlined />}
                            loading={systems.deletePending && systems.deletingSystem?.id === record.id}
                            onClick={() => systems.openDeleteModal(record)}
                        >
                            {t('button.delete')}
                        </Button>
                    </PermissionGuard>
                </Space>
            ),
        },
    ];

    return (
        <div>
            {systems.messageContextHolder}
            <PageHeader
                title={t('nav.systems')}
                subtitle={t('systems.subtitle')}
                actions={(
                    <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => systems.refetch()}>
                        {t('button.refresh')}
                    </Button>
                    <PermissionGuard permission="system:write">
                        <Button
                            type="primary"
                            icon={<PlusOutlined />}
                            data-testid="system-create-button"
                            onClick={systems.openCreateModal}
                        >
                            {t('button.create')}
                        </Button>
                    </PermissionGuard>
                    </Space>
                )}
            />

            {(systems.data?.items?.length ?? 0) === 0 && !systems.isLoading ? (
                <SetupGuideCard variant="systems" />
            ) : (
                <PageSurface flush={true}>
                    <Table<System>
                        columns={columns}
                        dataSource={systems.data?.items ?? []}
                        rowKey="id"
                        loading={systems.isLoading}
                        pagination={{
                            current: systems.page,
                            pageSize: systems.pageSize,
                            total: systems.data?.pagination?.total ?? 0,
                            showTotal: (total) => t('table.total', { total }),
                            onChange: (page, pageSize) => {
                                systems.setPage(page);
                                systems.setPageSize(pageSize);
                            },
                        }}
                        size="middle"
                    />
                </PageSurface>
            )}

            <Modal
                title={t('systems.modal.create_title')}
                open={systems.createOpen}
                onOk={() => {
                    void systems.submitCreate();
                }}
                onCancel={systems.closeCreateModal}
                confirmLoading={systems.createPending}
                forceRender={true}
                data-testid="system-create-modal"
            >
                <Form form={systems.form} layout="vertical" name="create-system">
                    <Form.Item
                        name="name"
                        label={t('table.name')}
                        rules={[
                            { required: true, message: t('systems.validation.name_required') },
                            { max: 15, message: t('systems.validation.name_max') },
                            {
                                pattern: RFC1035_PATTERN,
                                message: t('systems.validation.name_format'),
                            },
                        ]}
                    >
                        <Input placeholder={t('systems.name_placeholder')} maxLength={15} />
                    </Form.Item>
                    <Form.Item
                        label={t('table.description')}
                        extra={
                            <Space size="small" style={{ marginTop: 8 }}>
                                <Button
                                    type="link"
                                    size="small"
                                    icon={createPreviewMode ? <EditOutlined /> : <FileTextOutlined />}
                                    onClick={() => setCreatePreviewMode(!createPreviewMode)}
                                >
                                    {createPreviewMode ? '[Edit]' : '[Preview]'}
                                </Button>
                                <Upload
                                    accept=".md"
                                    showUploadList={false}
                                    beforeUpload={(file) => {
                                        const reader = new FileReader();
                                        reader.onload = (e) => {
                                            const text = e.target?.result as string;
                                            systems.form.setFieldValue('description', text);
                                        };
                                        reader.readAsText(file);
                                        return false;
                                    }}
                                >
                                    <Button type="link" size="small" icon={<UploadOutlined />}>
                                        [Upload .md file]
                                    </Button>
                                </Upload>
                            </Space>
                        }
                    >
                        <Form.Item noStyle shouldUpdate>
                            {(form) => (
                                <div className="markdown-preview" style={{ display: createPreviewMode ? 'block' : 'none', padding: '4px 11px', border: '1px solid #d9d9d9', borderRadius: 6, minHeight: 76, maxHeight: 152, overflowY: 'auto' }}>
                                    <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                                        {form.getFieldValue('description') || '*No content provided*'}
                                    </ReactMarkdown>
                                </div>
                            )}
                        </Form.Item>
                        <Form.Item name="description" noStyle hidden={createPreviewMode}>
                            <Input.TextArea rows={3} placeholder={t('systems.description_placeholder')} />
                        </Form.Item>
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('systems.modal.edit_title')}
                open={systems.editOpen}
                onOk={() => {
                    void systems.submitEdit();
                }}
                onCancel={systems.closeEditModal}
                confirmLoading={systems.updatePending}
                forceRender={true}
                data-testid="system-edit-modal"
            >
                <Form form={systems.editForm} layout="vertical" name="edit-system">
                    <Form.Item
                        label={t('table.description')}
                        extra={
                            <Space size="small" style={{ marginTop: 8 }}>
                                <Button
                                    type="link"
                                    size="small"
                                    icon={editPreviewMode ? <EditOutlined /> : <FileTextOutlined />}
                                    onClick={() => setEditPreviewMode(!editPreviewMode)}
                                >
                                    {editPreviewMode ? '[Edit]' : '[Preview]'}
                                </Button>
                                <Upload
                                    accept=".md"
                                    showUploadList={false}
                                    beforeUpload={(file) => {
                                        const reader = new FileReader();
                                        reader.onload = (e) => {
                                            const text = e.target?.result as string;
                                            systems.editForm.setFieldValue('description', text);
                                        };
                                        reader.readAsText(file);
                                        return false;
                                    }}
                                >
                                    <Button type="link" size="small" icon={<UploadOutlined />}>
                                        [Upload .md file]
                                    </Button>
                                </Upload>
                            </Space>
                        }
                    >
                        <Form.Item noStyle shouldUpdate>
                            {(form) => (
                                <div className="markdown-preview" style={{ display: editPreviewMode ? 'block' : 'none', padding: '4px 11px', border: '1px solid #d9d9d9', borderRadius: 6, minHeight: 76, maxHeight: 152, overflowY: 'auto' }}>
                                    <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                                        {form.getFieldValue('description') || '*No content provided*'}
                                    </ReactMarkdown>
                                </div>
                            )}
                        </Form.Item>
                        <Form.Item name="description" noStyle hidden={editPreviewMode}>
                            <Input.TextArea rows={3} placeholder={t('systems.edit_description_placeholder')} />
                        </Form.Item>
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={(
                    <Space>
                        <ExclamationCircleOutlined style={{ color: '#ff4d4f' }} />
                        {t('systems.delete_title')}
                    </Space>
                )}
                open={systems.deleteOpen}
                onOk={systems.submitDelete}
                onCancel={systems.closeDeleteModal}
                confirmLoading={systems.deletePending}
                okButtonProps={{
                    danger: true,
                    disabled: systems.deleteConfirmName !== systems.deletingSystem?.name,
                }}
                okText={t('button.delete')}
                forceRender={true}
                data-testid="system-delete-modal"
            >
                <Paragraph>
                    {t('systems.delete_confirm', { name: systems.deletingSystem?.name })}
                </Paragraph>
                <Paragraph type="secondary">
                    {t('systems.delete_type_name')}
                </Paragraph>
                <Input
                    value={systems.deleteConfirmName}
                    onChange={(e) => systems.setDeleteConfirmName(e.target.value)}
                    placeholder={systems.deletingSystem?.name}
                    status={systems.deleteConfirmName && systems.deleteConfirmName !== systems.deletingSystem?.name ? 'error' : undefined}
                />
            </Modal>

            <SystemMembersModal
                open={systems.membersOpen}
                onCancel={systems.closeMembersModal}
                systemId={systems.membersSystem?.id ?? null}
                systemName={systems.membersSystem?.name}
            />

            <Modal
                title={detailSystem?.name}
                open={detailOpen}
                onCancel={() => setDetailOpen(false)}
                footer={[
                    <Button key="close" onClick={() => setDetailOpen(false)}>
                        {t('button.close', 'Close')}
                    </Button>
                ]}
                forceRender                width={800}
            >
                {detailSystem?.description ? (
                    <div className="markdown-preview" style={{ padding: '16px', background: '#fafafa', borderRadius: 8, marginTop: 16 }}>
                        <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                            {detailSystem.description}
                        </ReactMarkdown>
                    </div>
                ) : (
                    <div style={{ paddingTop: 16 }}>
                        <ActionEmptyState
                            compact={true}
                            title={t('table.description')}
                            description={t('systems.description_placeholder')}
                            visual={<SystemsOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                        />
                    </div>
                )}
            </Modal>
        </div>
    );
}
