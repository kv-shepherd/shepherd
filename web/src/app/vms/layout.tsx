import type { Metadata } from 'next';
import ProtectedLayout from '@/components/layouts/ProtectedLayout';

export const metadata: Metadata = {
    title: 'Virtual Machines - KubeVirt Shepherd',
};

export default function VMsLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    return <ProtectedLayout>{children}</ProtectedLayout>;
}
