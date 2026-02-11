import type { Metadata } from 'next';
import ProtectedLayout from '@/components/layouts/ProtectedLayout';

export const metadata: Metadata = {
    title: 'Audit Log - KubeVirt Shepherd',
};

export default function AuditLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    return <ProtectedLayout>{children}</ProtectedLayout>;
}
