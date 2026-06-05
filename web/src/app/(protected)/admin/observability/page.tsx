import type { Metadata } from 'next';
import { ObservabilityPageContent } from '@/features/observability/components/ObservabilityPageContent';

export const metadata: Metadata = {
    title: 'Observability',
};

export default function AdminObservabilityPage() {
    return <ObservabilityPageContent />;
}
