import type { TFunction } from 'i18next';

const BUILT_IN_ROLE_CATALOG_KEYS: Record<string, string> = {
    Bootstrap: 'bootstrap',
    PlatformAdmin: 'platform_admin',
    SystemAdmin: 'system_admin',
    Approver: 'approver',
    Operator: 'operator',
    Viewer: 'viewer',
};

function builtInRoleCatalogTranslationKey(roleName: string) {
    return BUILT_IN_ROLE_CATALOG_KEYS[roleName]
        ?? roleName.replace(/[^a-zA-Z0-9]+/g, '_').replace(/^_+|_+$/g, '').toLowerCase();
}

export function localizeRoleLabel(
    t: TFunction,
    role: {
        name: string;
        built_in?: boolean;
        display_name?: string | null;
    }
) {
    if (!role.built_in) {
        return role.display_name?.trim() || role.name;
    }
    const catalogKey = builtInRoleCatalogTranslationKey(role.name);
    return t(`rbac.roles.catalog.${catalogKey}.label`, {
        defaultValue: role.display_name?.trim() || role.name,
    });
}

export function localizeRoleDescription(
    t: TFunction,
    role: {
        name: string;
        built_in?: boolean;
        description?: string | null;
    }
) {
    if (!role.built_in) {
        return role.description?.trim() || '';
    }
    const catalogKey = builtInRoleCatalogTranslationKey(role.name);
    return t(`rbac.roles.catalog.${catalogKey}.description`, {
        defaultValue: role.description?.trim() || '',
    });
}
