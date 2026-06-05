import type { Metadata } from 'next';
import { redirect } from 'next/navigation';

export const metadata: Metadata = {
    title: 'Observability',
};

export default function ObservabilityPage() {
    redirect('/admin/observability');
}
