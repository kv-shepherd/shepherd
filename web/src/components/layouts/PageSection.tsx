'use client';

import type { ReactNode } from 'react';
import { Card, Typography } from 'antd';
import type { CardProps } from 'antd';

const { Title, Text } = Typography;

interface PageHeaderProps {
    title: ReactNode;
    subtitle?: ReactNode;
    actions?: ReactNode;
}

interface PageSurfaceProps extends CardProps {
    flush?: boolean;
}

export function PageHeader({ title, subtitle, actions }: PageHeaderProps) {
    return (
        <div className="page-header">
            <div className="page-header__meta">
                <Title level={4} style={{ margin: 0 }}>
                    {title}
                </Title>
                {subtitle ? <Text type="secondary">{subtitle}</Text> : null}
            </div>
            {actions ? <div className="page-header__actions">{actions}</div> : null}
        </div>
    );
}

export function PageSurface({ children, className, flush = false, styles, ...props }: PageSurfaceProps) {
    const mergedClassName = ['app-shell-surface', 'page-surface', className].filter(Boolean).join(' ');

    return (
        <Card
            {...props}
            className={mergedClassName}
            styles={flush ? { ...styles, body: { padding: 0, ...(styles?.body ?? {}) } } : styles}
        >
            {children}
        </Card>
    );
}
