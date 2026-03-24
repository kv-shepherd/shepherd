import { describe, expect, it } from 'vitest';

import { getMenuRoutes, resolveMenuHref, type MenuRouteItem } from './appLayoutRoutes';

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

        expect(admin?.name).toBe('nav.admin');
        expect(admin?.path).toBe('/admin');
        expect(approvalTasks?.name).toBe('nav.approval_tasks');
    });

    it('resolves grouped menu hrefs to the first child route', () => {
        const route = getMenuRoutes(t, true);
        const admin = route.routes?.find((item: MenuRouteItem) => item.key === 'admin');

        expect(resolveMenuHref(admin ?? {})).toBe('/admin/approval-tasks');
    });
});
