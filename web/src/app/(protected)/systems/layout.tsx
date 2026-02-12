import type { Metadata } from 'next';

export const metadata: Metadata = {
    title: 'Systems',
};

export default function SystemsLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    return <>{children}</>;
}
