'use client';

/**
 * Protected layout wrapper — combines AuthGuard + AppLayout.
 *
 * AGENTS.md §3.5: Composition pattern. AuthGuard checks auth,
 * AppLayout provides shell. Both are client components.
 */
import AuthGuard from './AuthGuard';
import AppLayout from './AppLayout';
import { DisplayTimeZoneProvider } from '@/components/providers/DisplayTimeZoneProvider';

export default function ProtectedLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    return (
        <AuthGuard>
            <DisplayTimeZoneProvider>
                <AppLayout>{children}</AppLayout>
            </DisplayTimeZoneProvider>
        </AuthGuard>
    );
}
