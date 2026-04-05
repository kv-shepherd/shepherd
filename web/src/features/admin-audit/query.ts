export type AuditLogCategory = '' | 'requests' | 'approvals' | 'resource_changes' | 'system_tasks';

export type AuditLogFilters = {
    search: string;
    category: AuditLogCategory;
    action: string;
    approval_decision: string;
    actor: string;
    placement_advisory_code: string;
    placement_reason_code: string;
    resource_type: string;
    resource_id: string;
};

export function buildAuditLogQuery(page: number, pageSize: number, filters: AuditLogFilters) {
    return {
        page,
        per_page: pageSize,
        ...(filters.search ? { search: filters.search } : {}),
        ...(filters.category ? { category: filters.category } : {}),
        ...(filters.action ? { action: filters.action } : {}),
        ...(filters.approval_decision ? { approval_decision: filters.approval_decision } : {}),
        ...(filters.actor ? { actor: filters.actor } : {}),
        ...(filters.placement_advisory_code ? { placement_advisory_code: filters.placement_advisory_code } : {}),
        ...(filters.placement_reason_code ? { placement_reason_code: filters.placement_reason_code } : {}),
        ...(filters.resource_type ? { resource_type: filters.resource_type } : {}),
        ...(filters.resource_id ? { resource_id: filters.resource_id } : {}),
    };
}
