import type { Metadata } from 'next';

export const metadata: Metadata = {
    title: 'Login - KubeVirt Shepherd',
    description: 'Login to KubeVirt Shepherd management platform',
};

export default function AuthLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    // Auth pages have no sidebar/header — just centered content
    return <>{children}</>;
}
