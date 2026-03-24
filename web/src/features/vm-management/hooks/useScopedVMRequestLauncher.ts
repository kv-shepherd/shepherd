'use client';

import { useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useRef } from 'react';

interface OpenWizardPrefill {
    systemId?: string;
    serviceId?: string;
}

interface UseScopedVMRequestLauncherArgs {
    openWizard: (prefill?: OpenWizardPrefill) => void;
    openSimilarRequest: (vmId: string) => void;
    resumeDraft: () => void;
    canLaunchRequest?: boolean;
}

export function useScopedVMRequestLauncher({
    openWizard,
    openSimilarRequest,
    resumeDraft,
    canLaunchRequest = true,
}: UseScopedVMRequestLauncherArgs) {
    const router = useRouter();
    const searchParams = useSearchParams();
    const consumedLaunchSignatureRef = useRef('');

    useEffect(() => {
        const request = searchParams.get('request');
        if (request !== 'create') {
            return;
        }

        const draftMode = searchParams.get('draft') || undefined;
        const systemId = searchParams.get('system_id') || undefined;
        const serviceId = searchParams.get('service_id') || undefined;
        const sourceVmId = searchParams.get('source_vm_id') || undefined;
        const signature = `${request}|${draftMode ?? ''}|${systemId ?? ''}|${serviceId ?? ''}|${sourceVmId ?? ''}`;

        if (consumedLaunchSignatureRef.current === signature) {
            return;
        }

        if (!canLaunchRequest) {
            return;
        }

        if (draftMode === 'resume') {
            resumeDraft();
        } else if (sourceVmId) {
            openSimilarRequest(sourceVmId);
        } else {
            openWizard({ systemId, serviceId });
        }
        consumedLaunchSignatureRef.current = signature;
        router.replace('/vms', { scroll: false });
    }, [canLaunchRequest, openSimilarRequest, openWizard, resumeDraft, router, searchParams]);
}
