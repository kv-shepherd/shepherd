import React from 'react';
import Icon from '@ant-design/icons';
import type { GetProps } from 'antd';

type CustomIconComponentProps = GetProps<typeof Icon>;

const createMenuIcon = (SvgPath: React.FC) => {
    const Component = (props: Partial<CustomIconComponentProps>) => (
        <Icon
            component={() => (
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.75"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    width="1em"
                    height="1em"
                >
                    <SvgPath />
                </svg>
            )}
            {...props}
        />
    );
    Component.displayName = 'CustomMenuIcon';
    return Component;
};

// Main Menu Icons
export const DashboardIcon = createMenuIcon(() => (
    <>
        <rect x="3" y="3" width="7" height="9" rx="1.5" />
        <rect x="14" y="3" width="7" height="5" rx="1.5" />
        <rect x="14" y="12" width="7" height="9" rx="1.5" />
        <rect x="3" y="16" width="7" height="5" rx="1.5" />
    </>
));

export const SystemsIcon = createMenuIcon(() => (
    <>
        <rect x="2" y="4" width="20" height="4" rx="1.5" />
        <rect x="2" y="10" width="20" height="4" rx="1.5" />
        <rect x="2" y="16" width="20" height="4" rx="1.5" />
        <line x1="6" y1="6" x2="6.01" y2="6" strokeWidth="2.5" strokeLinecap="round" />
        <line x1="6" y1="12" x2="6.01" y2="12" strokeWidth="2.5" strokeLinecap="round" />
        <line x1="6" y1="18" x2="6.01" y2="18" strokeWidth="2.5" strokeLinecap="round" />
    </>
));

export const ServicesIcon = createMenuIcon(() => (
    <>
        <polygon points="12 2 22 8.5 22 15.5 12 22 2 15.5 2 8.5 12 2" />
        <line x1="12" y1="22" x2="12" y2="15.5" />
        <polyline points="22 8.5 12 15.5 2 8.5" />
        <polyline points="2 15.5 12 8.5 22 15.5" />
        <line x1="12" y1="2" x2="12" y2="8.5" />
    </>
));

export const VMsIcon = createMenuIcon(() => (
    <>
        <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
        <rect x="8" y="8" width="8" height="8" rx="1" />
        <line x1="8" y1="1" x2="8" y2="3" />
        <line x1="16" y1="1" x2="16" y2="3" />
        <line x1="8" y1="21" x2="8" y2="23" />
        <line x1="16" y1="21" x2="16" y2="23" />
        <line x1="1" y1="8" x2="3" y2="8" />
        <line x1="1" y1="16" x2="3" y2="16" />
        <line x1="21" y1="8" x2="23" y2="8" />
        <line x1="21" y1="16" x2="23" y2="16" />
    </>
));

export const RequestsIcon = createMenuIcon(() => (
    <>
        <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
        <polyline points="14 2 14 8 20 8" />
        <path d="m9 14 2 2 4-4" />
    </>
));

// Notifications
export const NotificationsIcon = createMenuIcon(() => (
    <>
        <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
        <path d="M13.73 21a2 2 0 0 1-3.46 0" />
    </>
));

// Admin Root Icon
export const AdminIcon = createMenuIcon(() => (
    <>
        <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
        <circle cx="12" cy="12" r="3" />
    </>
));

// Admin Submenu Icons
export const ApprovalTasksIcon = createMenuIcon(() => (
    <>
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
        <path d="m9 12 2 2 4-4" />
    </>
));

export const ClustersIcon = createMenuIcon(() => (
    <>
        <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
        <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
        <line x1="12" y1="22.08" x2="12" y2="12" />
    </>
));

export const NamespacesIcon = createMenuIcon(() => (
    <>
        <circle cx="12" cy="12" r="10" />
        <line x1="2" y1="12" x2="22" y2="12" />
        <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
    </>
));

export const TemplatesIcon = createMenuIcon(() => (
    <>
        <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </>
));

export const InstanceSizesIcon = createMenuIcon(() => (
    <>
        <rect x="3" y="8" width="18" height="8" rx="2" />
        <line x1="7" y1="8" x2="7" y2="12" />
        <line x1="11" y1="8" x2="11" y2="10" />
        <line x1="15" y1="8" x2="15" y2="12" />
        <line x1="3" y1="12" x2="21" y2="12" />
    </>
));

export const UsersIcon = createMenuIcon(() => (
    <>
        <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
        <circle cx="9" cy="7" r="4" />
        <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
        <path d="M16 3.13a4 4 0 0 1 0 7.75" />
    </>
));

export const RbacIcon = createMenuIcon(() => (
    <>
        <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
        <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </>
));

export const RateLimitsIcon = createMenuIcon(() => (
    <>
        <circle cx="12" cy="12" r="10" />
        <polyline points="12 6 12 12 15.5 15.5" />
        <line x1="12" y1="2" x2="12" y2="4" />
        <line x1="12" y1="20" x2="12" y2="22" />
        <line x1="2" y1="12" x2="4" y2="12" />
        <line x1="20" y1="12" x2="22" y2="12" />
    </>
));

export const AuthProvidersIcon = createMenuIcon(() => (
    <>
        <path d="M2 18v3c0 .6.4 1 1 1h4v-3h3v-3h2l1.4-1.4a6.5 6.5 0 1 0-4-4Z" />
        <circle cx="16.5" cy="7.5" r=".5" fill="currentColor" />
    </>
));

export const AuditIcon = createMenuIcon(() => (
    <>
        <rect x="4" y="4" width="16" height="16" rx="2" />
        <path d="M9 12h6" />
        <path d="M9 16h6" />
        <path d="M9 8h6" />
        <line x1="5" y1="4" x2="5" y2="20" />
    </>
));
