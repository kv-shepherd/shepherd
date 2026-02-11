'use client';

/**
 * Protected layout wrapper — combines AuthGuard + AppLayout.
 *
 * AGENTS.md §3.5: Composition pattern. AuthGuard checks auth,
 * AppLayout provides shell. Both are client components.
 */
import AuthGuard from './AuthGuard';
import AppLayout from './AppLayout';

export default function ProtectedLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    return (
        <AuthGuard>
            <AppLayout>{children}</AppLayout>
        </AuthGuard>
    );
}
