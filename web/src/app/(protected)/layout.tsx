import type { Metadata } from 'next';
import ProtectedLayout from '@/components/layouts/ProtectedLayout';

export const metadata: Metadata = {
    title: {
        template: '%s - KubeVirt Shepherd',
        default: 'KubeVirt Shepherd',
    },
};

export default function ProtectedRouteLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    return <ProtectedLayout>{children}</ProtectedLayout>;
}
