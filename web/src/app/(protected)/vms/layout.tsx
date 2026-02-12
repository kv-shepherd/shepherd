import type { Metadata } from 'next';

export const metadata: Metadata = {
    title: 'Virtual Machines',
};

export default function VMsLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    return <>{children}</>;
}
