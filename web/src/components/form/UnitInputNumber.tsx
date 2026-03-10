'use client';

import { Input, InputNumber, Space } from 'antd';
import type { InputNumberProps } from 'antd';

type UnitInputNumberProps = InputNumberProps & {
    unit: string;
    unitWidth?: number;
};

export function UnitInputNumber({
    unit,
    unitWidth = 64,
    style,
    ...props
}: UnitInputNumberProps) {
    return (
        <Space.Compact block>
            <InputNumber
                {...props}
                style={{ width: '100%', ...style }}
            />
            <Input
                readOnly
                tabIndex={-1}
                aria-hidden="true"
                value={unit}
                style={{
                    width: unitWidth,
                    textAlign: 'center',
                    pointerEvents: 'none',
                }}
            />
        </Space.Compact>
    );
}
