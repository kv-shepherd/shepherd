'use client';

import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Card, Descriptions, Divider, Space, Table, Tag, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

import { PermissionGuard } from '@/components/auth/PermissionGuard';
import { VMListTable } from '@/features/vm-management/components/VMListTable';
import { VMRequestWizard } from '@/features/vm-management/components/VMRequestWizard';
import { useVMManagementController } from '@/features/vm-management/hooks/useVMManagementController';

const { Title, Text } = Typography;

export default function VMsPage() {
    const { t } = useTranslation(['vm', 'common']);
    const vm = useVMManagementController({ t });
    const batchStatus = vm.batchStatus?.status;
    const batchCanRetry = batchStatus === 'FAILED' || batchStatus === 'PARTIAL_SUCCESS';
    const batchCanCancel = batchStatus === 'PENDING_APPROVAL' || batchStatus === 'IN_PROGRESS';

    return (
        <div>
            {vm.messageContextHolder}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('title')}</Title>
                    <Text type="secondary">{t('subtitle')}</Text>
                </div>
                <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => vm.refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <PermissionGuard permission="vm:create">
                        <Button type="primary" icon={<PlusOutlined />} onClick={vm.openWizard}>
                            {t('create_request')}
                        </Button>
                    </PermissionGuard>
                </Space>
            </div>

            <Card style={{ marginBottom: 16, borderRadius: 12 }}>
                <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Space direction="vertical" size={0}>
                        <Text strong>{t('batch.title')}</Text>
                        <Text type="secondary">{t('batch.subtitle')}</Text>
                    </Space>
                    <Space wrap>
                        <Tag color="blue">{t('batch.selected', { count: vm.selectedVMIDs.length })}</Tag>
                        <PermissionGuard permission="vm:operate">
                            <Button
                                onClick={() => vm.submitBatchPowerSelected('START')}
                                loading={vm.batchSubmitPending}
                                disabled={vm.batchRateLimited}
                            >
                                {t('batch.start_selected')}
                            </Button>
                        </PermissionGuard>
                        <PermissionGuard permission="vm:operate">
                            <Button
                                onClick={() => vm.submitBatchPowerSelected('STOP')}
                                loading={vm.batchSubmitPending}
                                disabled={vm.batchRateLimited}
                            >
                                {t('batch.stop_selected')}
                            </Button>
                        </PermissionGuard>
                        <PermissionGuard permission="vm:operate">
                            <Button
                                onClick={() => vm.submitBatchPowerSelected('RESTART')}
                                loading={vm.batchSubmitPending}
                                disabled={vm.batchRateLimited}
                            >
                                {t('batch.restart_selected')}
                            </Button>
                        </PermissionGuard>
                        <PermissionGuard permission="vm:delete">
                            <Button
                                danger
                                onClick={vm.submitBatchDeleteSelected}
                                loading={vm.batchSubmitPending}
                                disabled={vm.batchRateLimited}
                            >
                                {t('batch.delete_selected')}
                            </Button>
                        </PermissionGuard>
                    </Space>
                </Space>
                {vm.batchRateLimited && (
                    <div style={{ marginTop: 12 }}>
                        <Text type="warning">
                            {t('batch.rate_limited_wait', { seconds: vm.batchRetryAfterSeconds })}
                        </Text>
                    </div>
                )}
            </Card>

            <VMListTable
                t={t}
                vmData={vm.vmData}
                isLoading={vm.isLoading}
                page={vm.page}
                pageSize={vm.pageSize}
                onPageChange={(page, pageSize) => {
                    vm.setPage(page);
                    vm.setPageSize(pageSize);
                }}
                onStart={vm.startVM}
                onStop={vm.stopVM}
                onRestart={vm.restartVM}
                onConsole={vm.requestConsole}
                onDelete={vm.deleteVM}
                selectedRowKeys={vm.selectedVMIDs}
                onSelectionChange={vm.setSelectedVMIDs}
            />

            {vm.activeBatchID && (
                <Card style={{ marginTop: 16, borderRadius: 12 }}>
                    <div
                        role="status"
                        aria-live="polite"
                        data-testid="batch-status-live"
                        style={{ marginBottom: 12 }}
                    >
                        <Text type="secondary">
                            {t('batch.live_status_summary', {
                                batch_id: vm.activeBatchID,
                                status: vm.batchStatus?.status ?? '—',
                                success_count: vm.batchStatus?.success_count ?? 0,
                                failed_count: vm.batchStatus?.failed_count ?? 0,
                                pending_count: vm.batchStatus?.pending_count ?? 0,
                            })}
                        </Text>
                    </div>
                    <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                        <Space direction="vertical" size={0}>
                            <Text strong>{t('batch.current_batch')}</Text>
                            <Text type="secondary">{vm.activeBatchID}</Text>
                        </Space>
                        <Space>
                            <Button icon={<ReloadOutlined />} onClick={vm.refreshBatch} loading={vm.batchLoading}>
                                {t('batch.refresh_status')}
                            </Button>
                            <Button
                                onClick={vm.retryBatch}
                                disabled={!batchCanRetry || vm.batchRateLimited}
                                loading={vm.batchActionPending}
                            >
                                {t('batch.retry_failed')}
                            </Button>
                            <Button
                                danger
                                onClick={vm.cancelBatch}
                                disabled={!batchCanCancel || vm.batchRateLimited}
                                loading={vm.batchActionPending}
                            >
                                {t('batch.cancel_pending')}
                            </Button>
                            <Button onClick={vm.clearBatchTracking}>{t('batch.clear')}</Button>
                        </Space>
                    </Space>
                    <Divider />
                    {vm.lastBatchActionFeedback && (
                        <div style={{ marginBottom: 12 }}>
                            <Text>
                                {t(`batch.${vm.lastBatchActionFeedback.action}_submitted_detail`, {
                                    count: vm.lastBatchActionFeedback.affectedCount,
                                    tickets: vm.lastBatchActionFeedback.affectedTicketIDs.join(', '),
                                })}
                            </Text>
                        </div>
                    )}
                    <Descriptions bordered size="small" column={2}>
                        <Descriptions.Item label={t('batch.status')}>
                            <Tag color={batchStatus === 'COMPLETED' ? 'green' : batchStatus === 'FAILED' ? 'red' : 'blue'}>
                                {vm.batchStatus?.status || '—'}
                            </Tag>
                        </Descriptions.Item>
                        <Descriptions.Item label={t('batch.operation')}>
                            {vm.batchStatus?.operation || '—'}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('batch.child_count')}>
                            {vm.batchStatus?.child_count ?? 0}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('batch.success_count')}>
                            {vm.batchStatus?.success_count ?? 0}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('batch.failed_count')}>
                            {vm.batchStatus?.failed_count ?? 0}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('batch.pending_count')}>
                            {vm.batchStatus?.pending_count ?? 0}
                        </Descriptions.Item>
                    </Descriptions>
                    <Table
                        style={{ marginTop: 16 }}
                        rowKey="ticket_id"
                        loading={vm.batchLoading}
                        dataSource={vm.batchStatus?.children ?? []}
                        pagination={false}
                        columns={[
                            { title: t('batch.child.ticket'), dataIndex: 'ticket_id', key: 'ticket_id' },
                            { title: t('batch.child.resource'), dataIndex: 'resource_name', key: 'resource_name' },
                            {
                                title: t('batch.child.status'),
                                dataIndex: 'status',
                                key: 'status',
                                render: (status: string) => <Tag>{status}</Tag>,
                            },
                            { title: t('batch.child.attempt'), dataIndex: 'attempt_count', key: 'attempt_count' },
                            { title: t('batch.child.error'), dataIndex: 'last_error', key: 'last_error' },
                        ]}
                    />
                </Card>
            )}

            <VMRequestWizard
                t={t}
                open={vm.wizardOpen}
                step={vm.wizardStep}
                setStep={vm.setWizardStep}
                form={vm.form}
                wizardSteps={vm.wizardSteps}
                selectedSystemId={vm.selectedSystemId}
                onSystemChange={vm.onSystemChange}
                systemsData={vm.systemsData}
                servicesData={vm.servicesData}
                templatesData={vm.templatesData}
                sizesData={vm.sizesData}
                selectedTemplate={vm.selectedTemplate}
                selectedSize={vm.selectedSize}
                serviceIdValue={vm.serviceIdValue}
                namespaceValue={vm.namespaceValue}
                namespaceOptions={vm.namespaceOptions}
                reasonValue={vm.reasonValue}
                batchCountValue={vm.batchCountValue}
                isSubmitting={vm.createVMRequest.isPending || vm.batchSubmitPending}
                onCancel={vm.closeWizard}
                onNext={vm.goToNextWizardStep}
                onSubmit={vm.submitWizard}
            />
        </div>
    );
}
