import type { Metadata } from 'next';
import ProtectedLayout from '@/components/layouts/ProtectedLayout';

export const metadata: Metadata = {
    title: 'Systems - KubeVirt Shepherd',
};

export default function SystemsLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    return <ProtectedLayout>{children}</ProtectedLayout>;
}
