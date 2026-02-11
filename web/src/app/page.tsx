import { redirect } from 'next/navigation';

/**
 * Root page — redirects to dashboard.
 *
 * Unauthenticated users will be caught by AuthGuard
 * in the dashboard layout and redirected to /login.
 */
export default function RootPage() {
  redirect('/dashboard');
}
