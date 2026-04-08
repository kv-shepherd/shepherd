'use client';

import type { ReactNode } from 'react';
import { Space, Typography } from 'antd';

const { Text } = Typography;

interface ActionEmptyStateProps {
    title: ReactNode;
    description: ReactNode;
    visual?: ReactNode;
    actions?: ReactNode;
    compact?: boolean;
}

export function ActionEmptyState({
    actions,
    compact = false,
    description,
    title,
    visual,
}: ActionEmptyStateProps) {
    return (
        <div className={compact ? 'action-empty-state action-empty-state--compact' : 'action-empty-state'}>
            {visual ? (
                <div className="action-empty-state__visual">
                    {visual}
                </div>
            ) : null}
            <Space direction="vertical" size={compact ? 6 : 8} className="action-empty-state__copy">
                <Text strong className="action-empty-state__title">
                    {title}
                </Text>
                <Text type="secondary" className="action-empty-state__description">
                    {description}
                </Text>
                {actions ? <div className="action-empty-state__actions">{actions}</div> : null}
            </Space>
        </div>
    );
}
