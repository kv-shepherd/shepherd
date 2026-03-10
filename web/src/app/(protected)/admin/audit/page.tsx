'use client';

/**
 * /admin/audit — route shell only.
 *
 * Keep route pages thin; feature logic lives under web/src/features.
 */
import { AdminAuditContent } from '@/features/admin-audit/components/AdminAuditContent';
export { buildAuditLogQuery } from '@/features/admin-audit/query';

export default function AuditLogPage() {
    return <AdminAuditContent />;
}
