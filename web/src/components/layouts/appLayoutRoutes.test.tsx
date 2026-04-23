import { describe, expect, it } from 'vitest';

import {
    filterMenuSearchEntries,
    flattenMenuRoutes,
    getMenuRoutes,
    resolveMenuHref,
    type MenuRouteItem,
} from './appLayoutRoutes';
import { ADMIN_MENU_ROUTE_PERMISSIONS } from '@/lib/auth/permissions';

const t = (key: string) => key;

describe('getMenuRoutes', () => {
    it('keeps systems and services as direct formal navigation entries', () => {
        const route = getMenuRoutes(t, false);
        const systems = route.routes?.find((item: MenuRouteItem) => item.path === '/systems');
        const services = route.routes?.find((item: MenuRouteItem) => item.path === '/services');

        expect(systems?.name).toBe('nav.systems');
        expect(services?.name).toBe('nav.services');
    });

    it('keeps vm inventory and my requests as direct formal navigation entries', () => {
        const route = getMenuRoutes(t, false);
        const vms = route.routes?.find((item: MenuRouteItem) => item.path === '/vms');
        const myRequests = route.routes?.find((item: MenuRouteItem) => item.path === '/tickets');

        expect(vms?.name).toBe('nav.vms');
        expect(myRequests?.name).toBe('nav.my_requests');
    });

    it('keeps notifications route alive but hidden from the side menu', () => {
        const route = getMenuRoutes(t, false);
        const notifications = route.routes?.find((item: MenuRouteItem) => item.path === '/notifications');

        expect(notifications?.hideInMenu).toBe(true);
    });

    it('surfaces admin approval tasks as a dedicated built-in route', () => {
        const route = getMenuRoutes(t, true);
        const admin = route.routes?.find((item: MenuRouteItem) => item.key === 'admin');
        const approvalTasks = admin?.routes?.find((item: MenuRouteItem) => item.path === '/admin/approval-tasks');
        const rateLimits = admin?.routes?.find((item: MenuRouteItem) => item.path === '/admin/rate-limits');

        expect(admin?.name).toBe('nav.admin');
        expect(admin?.path).toBe('/admin');
        expect(approvalTasks?.name).toBe('nav.approval_tasks');
        expect(rateLimits?.name).toBe('nav.rate_limits');
    });

    it('resolves grouped menu hrefs to the first child route', () => {
        const route = getMenuRoutes(t, true);
        const admin = route.routes?.find((item: MenuRouteItem) => item.key === 'admin');

        expect(resolveMenuHref(admin ?? {})).toBe('/admin/approval-tasks');
    });

    it('flattens visible leaf routes for global navigation search', () => {
        const route = getMenuRoutes(t, true);
        const entries = flattenMenuRoutes(route.routes as MenuRouteItem[]);

        expect(entries).toEqual(
            expect.arrayContaining([
                expect.objectContaining({
                    path: '/systems',
                    label: 'nav.systems',
                }),
                expect.objectContaining({
                    path: '/admin/templates',
                    label: 'nav.templates',
                    groupLabel: 'nav.admin',
                }),
            ]),
        );
    });

    it('filters flattened routes using case-insensitive fuzzy terms', () => {
        const route = getMenuRoutes(t, true);
        const entries = flattenMenuRoutes(route.routes as MenuRouteItem[]);
        const results = filterMenuSearchEntries(entries, 'admin templ');

        expect(results).toEqual([
            expect.objectContaining({
                path: '/admin/templates',
            }),
        ]);
    });

    it('filters cluster admin routes by canonical permission groups', () => {
        const route = getMenuRoutes(t, (permissions) => permissions.includes('cluster:read'));
        const admin = route.routes?.find((item: MenuRouteItem) => item.key === 'admin');

        expect(admin?.routes).toEqual([
            expect.objectContaining({ path: '/admin/clusters' }),
        ]);
        expect(resolveMenuHref(admin ?? {})).toBe('/admin/clusters');
    });

    it('keeps namespaces visible for namespace-scoped admins', () => {
        const route = getMenuRoutes(t, (permissions) => permissions.includes('namespace:read'));
        const admin = route.routes?.find((item: MenuRouteItem) => item.key === 'admin');

        expect(admin?.routes).toEqual([
            expect.objectContaining({ path: '/admin/namespaces' }),
        ]);
        expect(resolveMenuHref(admin ?? {})).toBe('/admin/namespaces');
    });

    it('keeps the users route visible for RBAC read-only operators', () => {
        const route = getMenuRoutes(t, (permissions) => permissions.includes('rbac:read'));
        const admin = route.routes?.find((item: MenuRouteItem) => item.key === 'admin');

        expect(admin?.routes).toEqual(expect.arrayContaining([
            expect.objectContaining({ path: '/admin/users' }),
            expect.objectContaining({ path: '/admin/rbac' }),
        ]));
    });

    it('renders every admin child route when all canonical permissions are available', () => {
        const route = getMenuRoutes(t, true);
        const admin = route.routes?.find((item: MenuRouteItem) => item.key === 'admin');
        const adminPaths = admin?.routes?.map((item: MenuRouteItem) => item.path) ?? [];

        expect(adminPaths).toHaveLength(Object.keys(ADMIN_MENU_ROUTE_PERMISSIONS).length);
        expect(adminPaths).toEqual(expect.arrayContaining([
            '/admin/approval-tasks',
            '/admin/clusters',
            '/admin/namespaces',
            '/admin/templates',
            '/admin/instance-sizes',
            '/admin/users',
            '/admin/rbac',
            '/admin/rate-limits',
            '/admin/auth-providers',
            '/admin/audit',
        ]));
    });
});
