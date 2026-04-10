'use client';

import type { CSSProperties, ReactNode } from 'react';
import { Card, Space, Typography } from 'antd';

const { Text } = Typography;

interface SummaryMetricCardProps {
    title: ReactNode;
    value: ReactNode;
    description: ReactNode;
    visual?: ReactNode;
    action?: ReactNode;
    accentColor: string;
    surfaceColor: string;
}

export function SummaryMetricCard({
    accentColor,
    action,
    description,
    surfaceColor,
    title,
    value,
    visual,
}: SummaryMetricCardProps) {
    return (
        <Card
            size="small"
            className="summary-metric-card"
            style={{
                '--summary-card-accent': accentColor,
                '--summary-card-surface': surfaceColor,
            } as CSSProperties}
        >
            <div className="summary-metric-card__layout">
                <Space direction="vertical" size={6} className="summary-metric-card__content">
                    <Text strong className="summary-metric-card__title">{title}</Text>
                    <Text strong className="summary-metric-card__value">
                        {value}
                    </Text>
                    <Text type="secondary" className="summary-metric-card__description">{description}</Text>
                    {action ? <div className="summary-metric-card__actions">{action}</div> : null}
                </Space>
                {visual ? (
                    <div className="summary-metric-card__visual">
                        {visual}
                    </div>
                ) : null}
            </div>
        </Card>
    );
}
