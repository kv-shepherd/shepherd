import type { Metadata } from 'next';
import ProtectedLayout from '@/components/layouts/ProtectedLayout';

export const metadata: Metadata = {
    title: 'Dashboard - KubeVirt Shepherd',
};

export default function DashboardLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    return <ProtectedLayout>{children}</ProtectedLayout>;
}
