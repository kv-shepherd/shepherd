'use client';

import { Form, message } from 'antd';
import type { TFunction } from 'i18next';
import { useEffect, useMemo, useRef, useState } from 'react';

import type { ApiErrorResponse } from '@/hooks/useApiQuery';
import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';

import type {
    ApprovalTicketResponse,
    DeleteVMResponse,
    InstanceSize,
    InstanceSizeList,
    ServiceList,
    SystemList,
    Template,
    TemplateList,
    VMBatchActionResponse,
    VMBatchPowerAction,
    VMBatchPowerRequest,
    VMBatchStatusResponse,
    VMBatchSubmitRequest,
    VMBatchSubmitResponse,
    VMConsoleRequestResponse,
    VMCreateRequest,
    VMList,
    VMRequestContext,
    VMVNCSessionResponse,
} from '../types';

interface UseVMManagementControllerArgs {
    t: TFunction;
}

type VMCreateFormValues = VMCreateRequest & { batch_count?: number };

const TERMINAL_BATCH_STATUSES = new Set([
    'COMPLETED',
    'PARTIAL_SUCCESS',
    'FAILED',
    'CANCELLED',
]);

const noVNCEntry = process.env.NEXT_PUBLIC_NOVNC_ENTRY ?? '/novnc/vnc.html';

type BatchActionKind = 'retry' | 'cancel';

interface BatchActionFeedback {
    action: BatchActionKind;
    affectedCount: number;
    affectedTicketIDs: string[];
}

const buildNoVNCURL = (websocketPath: string): string => {
    const cleaned = websocketPath.startsWith('/') ? websocketPath.slice(1) : websocketPath;
    return `${noVNCEntry}?path=${encodeURIComponent(cleaned)}`;
};

const parseBatchIDFromStatusURL = (statusURL: string, fallback: string): string => {
    const trimmed = statusURL.trim();
    if (trimmed === '') {
        return fallback;
    }
    const segments = trimmed.split('/').filter(Boolean);
    const candidate = segments.at(-1);
    return candidate && candidate.trim() !== '' ? candidate : fallback;
};

const normalizeRetryAfterSeconds = (value: unknown): number => {
    const n = Number(value);
    if (!Number.isFinite(n)) {
        return 0;
    }
    return Math.max(0, Math.ceil(n));
};

const extractRetryAfterSeconds = (error: ApiErrorResponse): number => {
    if (typeof error.retry_after_seconds === 'number') {
        return normalizeRetryAfterSeconds(error.retry_after_seconds);
    }
    const params = error.params as Record<string, unknown> | undefined;
    if (params && Object.prototype.hasOwnProperty.call(params, 'retry_after_seconds')) {
        return normalizeRetryAfterSeconds(params.retry_after_seconds);
    }
    return 0;
};

const summarizeTicketIDs = (ids: string[]): string => {
    if (ids.length <= 3) {
        return ids.join(', ');
    }
    const remain = ids.length - 3;
    return `${ids.slice(0, 3).join(', ')} +${remain}`;
};

export function useVMManagementController({ t }: UseVMManagementControllerArgs) {
    const [messageApi, messageContextHolder] = message.useMessage();
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [wizardOpen, setWizardOpen] = useState(false);
    const [wizardStep, setWizardStep] = useState(0);
    const [selectedSystemId, setSelectedSystemId] = useState('');
    const [selectedVMIDs, setSelectedVMIDs] = useState<string[]>([]);
    const [activeBatchID, setActiveBatchID] = useState('');
    const [activeBatchStatusURL, setActiveBatchStatusURL] = useState('');
    const [batchAutoPolling, setBatchAutoPolling] = useState(true);
    const [batchPollingIntervalMs, setBatchPollingIntervalMs] = useState(2000);
    const [batchRateLimitUntilMs, setBatchRateLimitUntilMs] = useState(0);
    const [nowMs, setNowMs] = useState(() => Date.now());
    const [lastBatchActionFeedback, setLastBatchActionFeedback] = useState<BatchActionFeedback | null>(null);
    const batchActionTargetIDsRef = useRef<string[]>([]);
    const [form] = Form.useForm<VMCreateFormValues>();

    const selectedTemplateId = Form.useWatch('template_id', form);
    const selectedSizeId = Form.useWatch('instance_size_id', form);
    const namespaceValue = Form.useWatch('namespace', form);
    const reasonValue = Form.useWatch('reason', form);
    const serviceIdValue = Form.useWatch('service_id', form);
    const batchCountValue = Form.useWatch('batch_count', form) ?? 1;

    const vmListQuery = useApiGet<VMList>(
        ['vms', page, pageSize],
        () => api.GET('/vms', { params: { query: { page, per_page: pageSize } } })
    );

    const systemsQuery = useApiGet<SystemList>(
        ['systems', 'vm-wizard'],
        () => api.GET('/systems', { params: { query: { per_page: 100 } } }),
        { enabled: wizardOpen }
    );

    const servicesQuery = useApiGet<ServiceList>(
        ['services', selectedSystemId, 'vm-wizard'],
        () => api.GET('/systems/{system_id}/services', {
            params: { path: { system_id: selectedSystemId }, query: { per_page: 100 } },
        }),
        { enabled: wizardOpen && Boolean(selectedSystemId) }
    );

    const requestContextQuery = useApiGet<VMRequestContext>(
        ['vm-request-context'],
        () => api.GET('/vms/request-context'),
        { enabled: wizardOpen }
    );

    // Backward-compatible fallback for environments where request-context is unavailable.
    const templatesFallbackQuery = useApiGet<TemplateList>(
        ['templates', 'vm-wizard-fallback'],
        () => api.GET('/templates'),
        { enabled: wizardOpen && requestContextQuery.isError }
    );

    const instanceSizesFallbackQuery = useApiGet<InstanceSizeList>(
        ['instance-sizes', 'vm-wizard-fallback'],
        () => api.GET('/instance-sizes'),
        { enabled: wizardOpen && requestContextQuery.isError }
    );

    const batchStatusQuery = useApiGet<VMBatchStatusResponse>(
        ['vm-batch', activeBatchID, activeBatchStatusURL],
        () => api.GET('/vms/batch/{batch_id}', {
            params: { path: { batch_id: activeBatchID } },
        }),
        {
            enabled: Boolean(activeBatchID),
            retry: 3,
            retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 30000),
            refetchInterval: (query) => {
                if (!batchAutoPolling) {
                    return false;
                }
                const status = (query.state.data as VMBatchStatusResponse | undefined)?.status;
                if (status && TERMINAL_BATCH_STATUSES.has(status)) {
                    return false;
                }
                return batchPollingIntervalMs;
            },
        }
    );

    const selectedTemplate = useMemo(() => {
        const templates = requestContextQuery.data?.templates ?? templatesFallbackQuery.data?.items ?? [];
        return templates.find((item: Template) => item.id === selectedTemplateId);
    }, [selectedTemplateId, requestContextQuery.data, templatesFallbackQuery.data]);

    const selectedSize = useMemo(() => {
        const sizes = requestContextQuery.data?.instance_sizes ?? instanceSizesFallbackQuery.data?.items ?? [];
        return sizes.find((item: InstanceSize) => item.id === selectedSizeId);
    }, [selectedSizeId, requestContextQuery.data, instanceSizesFallbackQuery.data]);

    const templatesData = useMemo<TemplateList | undefined>(() => {
        if (requestContextQuery.data) {
            return { items: requestContextQuery.data.templates ?? [] };
        }
        return templatesFallbackQuery.data;
    }, [requestContextQuery.data, templatesFallbackQuery.data]);

    const sizesData = useMemo<InstanceSizeList | undefined>(() => {
        if (requestContextQuery.data) {
            return { items: requestContextQuery.data.instance_sizes ?? [] };
        }
        return instanceSizesFallbackQuery.data;
    }, [requestContextQuery.data, instanceSizesFallbackQuery.data]);

    useEffect(() => {
        if (batchRateLimitUntilMs <= Date.now()) {
            return;
        }
        const timer = window.setInterval(() => {
            const now = Date.now();
            setNowMs(now);
            setBatchRateLimitUntilMs((current) => (current > 0 && now >= current ? 0 : current));
        }, 1000);
        return () => window.clearInterval(timer);
    }, [batchRateLimitUntilMs]);

    const batchRetryAfterSeconds = Math.max(0, Math.ceil((batchRateLimitUntilMs - nowMs) / 1000));
    const batchRateLimited = batchRetryAfterSeconds > 0;

    const setBatchRateLimitCooldown = (seconds: number) => {
        const normalized = normalizeRetryAfterSeconds(seconds);
        if (normalized <= 0) {
            return false;
        }
        const now = Date.now();
        setNowMs(now);
        setBatchRateLimitUntilMs(now + normalized * 1000);
        messageApi.warning(t('batch.rate_limited_wait', { seconds: normalized }));
        return true;
    };

    const trackBatchSubmission = (resp: VMBatchSubmitResponse) => {
        const trackedBatchID = parseBatchIDFromStatusURL(resp.status_url, resp.batch_id);
        setActiveBatchID(trackedBatchID);
        setActiveBatchStatusURL(resp.status_url);
        setBatchAutoPolling(true);
        setLastBatchActionFeedback(null);
        const intervalSeconds = normalizeRetryAfterSeconds(resp.retry_after_seconds);
        setBatchPollingIntervalMs(intervalSeconds > 0 ? intervalSeconds * 1000 : 2000);
        setBatchRateLimitUntilMs(0);
        setNowMs(Date.now());
    };

    const pickBatchActionTargets = (action: BatchActionKind): string[] => {
        const children = batchStatusQuery.data?.children ?? [];
        if (action === 'retry') {
            return children
                .filter((child) => child.status === 'FAILED' || child.status === 'REJECTED')
                .map((child) => child.ticket_id);
        }
        return children
            .filter((child) => child.status === 'PENDING')
            .map((child) => child.ticket_id);
    };

    const resolveAffectedTicketIDs = (
        resp: VMBatchActionResponse,
        fallbackIDs: string[],
    ): string[] => {
        const fromResponse = (resp.affected_ticket_ids ?? []).map((id) => id.trim()).filter(Boolean);
        if (fromResponse.length > 0) {
            return fromResponse;
        }
        const affectedCount = Math.max(0, Number(resp.affected_count ?? 0));
        if (affectedCount <= 0) {
            return [];
        }
        return fallbackIDs.slice(0, Math.min(affectedCount, fallbackIDs.length));
    };

    const onBatchMutationRateLimit = (err: ApiErrorResponse): boolean => {
        if (err.code !== 'BATCH_RATE_LIMITED') {
            return false;
        }
        return setBatchRateLimitCooldown(extractRetryAfterSeconds(err));
    };

    const createVMRequest = useApiMutation<VMCreateRequest, ApprovalTicketResponse>(
        (req) => api.POST('/vms/request', { body: req }),
        {
            invalidateKeys: [['vms'], ['approvals']],
            onSuccess: () => {
                messageApi.success(t('request_submitted'));
                setWizardOpen(false);
                setWizardStep(0);
                setSelectedSystemId('');
                form.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const submitCreateBatch = useApiMutation<VMBatchSubmitRequest, VMBatchSubmitResponse>(
        (req) => api.POST('/approvals/batch', { body: req }),
        {
            invalidateKeys: [['vms'], ['approvals']],
            onSuccess: (resp) => {
                trackBatchSubmission(resp);
                setWizardOpen(false);
                setWizardStep(0);
                setSelectedSystemId('');
                form.resetFields();
                messageApi.success(t('batch.submitted', { batch_id: resp.batch_id }));
            },
            onError: (err) => {
                if (onBatchMutationRateLimit(err)) {
                    return;
                }
                messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const submitVMBatch = useApiMutation<VMBatchSubmitRequest, VMBatchSubmitResponse>(
        (req) => api.POST('/vms/batch', { body: req }),
        {
            invalidateKeys: [['vms'], ['approvals']],
            onSuccess: (resp) => {
                trackBatchSubmission(resp);
                messageApi.success(t('batch.submitted', { batch_id: resp.batch_id }));
            },
            onError: (err) => {
                if (onBatchMutationRateLimit(err)) {
                    return;
                }
                messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const submitVMBatchPower = useApiMutation<VMBatchPowerRequest, VMBatchSubmitResponse>(
        (req) => api.POST('/vms/batch/power', { body: req }),
        {
            invalidateKeys: [['vms']],
            onSuccess: (resp) => {
                trackBatchSubmission(resp);
                messageApi.success(t('batch.submitted', { batch_id: resp.batch_id }));
            },
            onError: (err) => {
                if (onBatchMutationRateLimit(err)) {
                    return;
                }
                messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const retryBatchMutation = useApiMutation<string, VMBatchActionResponse>(
        (batchID) => api.POST('/vms/batch/{batch_id}/retry', {
            params: { path: { batch_id: batchID } },
        }),
        {
            invalidateKeys: [['vm-batch', activeBatchID, activeBatchStatusURL], ['vms']],
            onSuccess: (resp) => {
                setBatchAutoPolling(true);
                const affectedTicketIDs = resolveAffectedTicketIDs(resp, batchActionTargetIDsRef.current);
                setLastBatchActionFeedback({
                    action: 'retry',
                    affectedCount: resp.affected_count,
                    affectedTicketIDs,
                });
                if (affectedTicketIDs.length > 0) {
                    messageApi.success(t('batch.retry_submitted_detail', {
                        count: resp.affected_count,
                        tickets: summarizeTicketIDs(affectedTicketIDs),
                    }));
                } else {
                    messageApi.success(t('batch.retry_submitted'));
                }
            },
            onError: (err) => {
                if (onBatchMutationRateLimit(err)) {
                    return;
                }
                messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const cancelBatchMutation = useApiMutation<string, VMBatchActionResponse>(
        (batchID) => api.POST('/vms/batch/{batch_id}/cancel', {
            params: { path: { batch_id: batchID } },
        }),
        {
            invalidateKeys: [['vm-batch', activeBatchID, activeBatchStatusURL], ['vms']],
            onSuccess: (resp) => {
                setBatchAutoPolling(true);
                const affectedTicketIDs = resolveAffectedTicketIDs(resp, batchActionTargetIDsRef.current);
                setLastBatchActionFeedback({
                    action: 'cancel',
                    affectedCount: resp.affected_count,
                    affectedTicketIDs,
                });
                if (affectedTicketIDs.length > 0) {
                    messageApi.success(t('batch.cancel_submitted_detail', {
                        count: resp.affected_count,
                        tickets: summarizeTicketIDs(affectedTicketIDs),
                    }));
                } else {
                    messageApi.success(t('batch.cancel_submitted'));
                }
            },
            onError: (err) => {
                if (onBatchMutationRateLimit(err)) {
                    return;
                }
                messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const startVM = useApiAction<string>(
        (vmId) => api.POST('/vms/{vm_id}/start', { params: { path: { vm_id: vmId } } }),
        {
            invalidateKeys: [['vms']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const stopVM = useApiAction<string>(
        (vmId) => api.POST('/vms/{vm_id}/stop', { params: { path: { vm_id: vmId } } }),
        {
            invalidateKeys: [['vms']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const restartVM = useApiAction<string>(
        (vmId) => api.POST('/vms/{vm_id}/restart', { params: { path: { vm_id: vmId } } }),
        {
            invalidateKeys: [['vms']],
            onSuccess: () => messageApi.success(t('common:message.success')),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const requestConsole = useApiMutation<string, VMConsoleRequestResponse>(
        (vmId) => api.POST('/vms/{vm_id}/console/request', {
            params: { path: { vm_id: vmId } },
        }),
        {
            invalidateKeys: [['approvals']],
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const deleteVM = useApiMutation<{ vmId: string; vmName: string }, DeleteVMResponse>(
        ({ vmId, vmName }) => api.DELETE('/vms/{vm_id}', {
            params: {
                path: { vm_id: vmId },
                query: { confirm: true, confirm_name: vmName },
            },
        }),
        {
            invalidateKeys: [['vms'], ['approvals']],
            onSuccess: (resp) => messageApi.success(t('delete_request_submitted', { ticket_id: resp.ticket_id })),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const wizardSteps = [
        { title: t('wizard.step.service') },
        { title: t('wizard.step.template') },
        { title: t('wizard.step.size') },
        { title: t('wizard.step.config') },
        { title: t('wizard.step.confirm') },
    ];

    const openWizard = () => {
        setWizardOpen(true);
        setWizardStep(0);
        setSelectedSystemId('');
        form.resetFields();
        form.setFieldValue('batch_count', 1);
    };

    const closeWizard = () => {
        setWizardOpen(false);
        setWizardStep(0);
        setSelectedSystemId('');
        form.resetFields();
    };

    const onSystemChange = (systemId: string) => {
        setSelectedSystemId(systemId);
        form.setFieldValue('service_id', undefined);
    };

    const goToNextWizardStep = async () => {
        const fieldsByStep: Array<Array<keyof VMCreateFormValues>> = [
            ['service_id'],
            ['template_id'],
            ['instance_size_id'],
            ['namespace', 'reason', 'batch_count'],
            [],
        ];

        const fields = fieldsByStep[wizardStep] ?? [];
        if (fields.length === 0) {
            setWizardStep((step) => step + 1);
            return;
        }

        try {
            await form.validateFields(fields);
            setWizardStep((step) => step + 1);
        } catch {
            // Ant Form shows validation errors in place.
        }
    };

    const submitWizard = () => {
        const values = form.getFieldsValue();
        const singlePayload: VMCreateRequest = {
            service_id: values.service_id,
            template_id: values.template_id,
            instance_size_id: values.instance_size_id,
            namespace: values.namespace,
            reason: values.reason,
        };
        const batchCount = Number(values.batch_count ?? 1);

        if (!Number.isFinite(batchCount) || batchCount <= 1) {
            createVMRequest.mutate(singlePayload);
            return;
        }
        if (batchRateLimited) {
            messageApi.warning(t('batch.rate_limited_wait', { seconds: batchRetryAfterSeconds }));
            return;
        }

        const batchPayload: VMBatchSubmitRequest = {
            operation: 'CREATE',
            reason: singlePayload.reason,
            items: Array.from({ length: batchCount }, () => ({
                service_id: singlePayload.service_id,
                template_id: singlePayload.template_id,
                instance_size_id: singlePayload.instance_size_id,
                namespace: singlePayload.namespace,
                reason: singlePayload.reason,
            })),
        };
        submitCreateBatch.mutate(batchPayload);
    };

    const submitBatchDeleteSelected = () => {
        if (batchRateLimited) {
            messageApi.warning(t('batch.rate_limited_wait', { seconds: batchRetryAfterSeconds }));
            return;
        }
        if (selectedVMIDs.length === 0) {
            messageApi.warning(t('batch.no_selection'));
            return;
        }
        submitVMBatch.mutate({
            operation: 'DELETE',
            reason: t('batch.delete_reason'),
            items: selectedVMIDs.map((vmID) => ({
                vm_id: vmID,
                reason: t('batch.delete_reason'),
            })),
        });
    };

    const submitBatchPowerSelected = (operation: VMBatchPowerAction) => {
        if (batchRateLimited) {
            messageApi.warning(t('batch.rate_limited_wait', { seconds: batchRetryAfterSeconds }));
            return;
        }
        if (selectedVMIDs.length === 0) {
            messageApi.warning(t('batch.no_selection'));
            return;
        }
        submitVMBatchPower.mutate({
            operation,
            reason: t('batch.power_reason', { operation }),
            items: selectedVMIDs.map((vmID) => ({
                vm_id: vmID,
                reason: t('batch.power_reason', { operation }),
            })),
        });
    };

    const openVNCTab = async (vmID: string): Promise<boolean> => {
        const { data, error } = await api.GET('/vms/{vm_id}/vnc', {
            params: { path: { vm_id: vmID } },
        });
        const session = data as VMVNCSessionResponse | undefined;
        if (error || !session?.websocket_path) {
            messageApi.error(t('console.unavailable'));
            return false;
        }

        const noVNCURL = buildNoVNCURL(session.websocket_path);
        window.open(noVNCURL, '_blank', 'noopener,noreferrer');
        messageApi.success(t('console.opened'));
        return true;
    };

    const fetchVMDetail = async (vmID: string) => {
        const { data, error } = await api.GET('/vms/{vm_id}', {
            params: { path: { vm_id: vmID } },
        });
        if (error || !data) {
            messageApi.error(t('common:message.error'));
            return null;
        }
        return data;
    };

    return {
        messageContextHolder,
        page,
        pageSize,
        setPage,
        setPageSize,
        wizardOpen,
        wizardStep,
        setWizardStep,
        form,
        selectedSystemId,
        selectedTemplate,
        selectedSize,
        namespaceValue,
        reasonValue,
        serviceIdValue,
        batchCountValue,
        wizardSteps,
        vmData: vmListQuery.data,
        isLoading: vmListQuery.isLoading,
        refetch: vmListQuery.refetch,
        systemsData: systemsQuery.data,
        servicesData: servicesQuery.data,
        templatesData,
        sizesData,
        namespaceOptions: requestContextQuery.data?.namespaces ?? [],
        createVMRequest,
        openWizard,
        closeWizard,
        onSystemChange,
        goToNextWizardStep,
        submitWizard,
        selectedVMIDs,
        setSelectedVMIDs,
        activeBatchID,
        activeBatchStatusURL,
        batchStatus: batchStatusQuery.data,
        batchLoading: batchStatusQuery.isLoading,
        batchRateLimited,
        batchRetryAfterSeconds,
        lastBatchActionFeedback,
        refreshBatch: () => {
            if (!activeBatchID) {
                return;
            }
            setBatchAutoPolling(true);
            void batchStatusQuery.refetch();
        },
        clearBatchTracking: () => {
            setActiveBatchID('');
            setActiveBatchStatusURL('');
            setBatchAutoPolling(false);
            setLastBatchActionFeedback(null);
        },
        retryBatch: () => {
            if (!activeBatchID) {
                return;
            }
            if (batchRateLimited) {
                messageApi.warning(t('batch.rate_limited_wait', { seconds: batchRetryAfterSeconds }));
                return;
            }
            batchActionTargetIDsRef.current = pickBatchActionTargets('retry');
            retryBatchMutation.mutate(activeBatchID);
        },
        cancelBatch: () => {
            if (!activeBatchID) {
                return;
            }
            if (batchRateLimited) {
                messageApi.warning(t('batch.rate_limited_wait', { seconds: batchRetryAfterSeconds }));
                return;
            }
            batchActionTargetIDsRef.current = pickBatchActionTargets('cancel');
            cancelBatchMutation.mutate(activeBatchID);
        },
        submitBatchDeleteSelected,
        submitBatchPowerSelected,
        batchSubmitPending: submitVMBatch.isPending || submitVMBatchPower.isPending || submitCreateBatch.isPending,
        batchActionPending: retryBatchMutation.isPending || cancelBatchMutation.isPending,
        startVM: (vmId: string) => startVM.mutate(vmId),
        stopVM: (vmId: string) => stopVM.mutate(vmId),
        restartVM: (vmId: string) => restartVM.mutate(vmId),
        requestConsole: async (vmId: string) => {
            const vm = await fetchVMDetail(vmId);
            if (!vm) {
                return;
            }
            requestConsole.mutate(vmId, {
                onSuccess: (resp) => {
                    if (resp.status === 'APPROVED') {
                        void openVNCTab(vmId);
                        return;
                    }
                    if (resp.status === 'PENDING_APPROVAL') {
                        messageApi.info(t('console.pending_approval'));
                        void api.GET('/vms/{vm_id}/console/status', {
                            params: { path: { vm_id: vmId } },
                        }).then(({ data }) => {
                            if (data?.status === 'APPROVED') {
                                void openVNCTab(vmId);
                            }
                        });
                        return;
                    }
                    messageApi.warning(t('console.unavailable'));
                },
            });
        },
        deleteVM: async (vmId: string, vmName: string) => {
            const vm = await fetchVMDetail(vmId);
            deleteVM.mutate({ vmId, vmName: vm?.name || vmName });
        },
    };
}
