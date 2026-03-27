import { Modal } from 'antd';
import type { ModalProps } from 'antd';

type WorkbenchDetailModalProps = ModalProps & {
    contentMinWidth?: number | string;
    maxBodyHeight?: string;
    topOffset?: number;
    bodyPaddingRight?: number;
    canvasClassName?: string;
};

function formatContentMinWidth(value: number | string): string | number {
    return typeof value === 'number' ? `${value}px` : value;
}

export function WorkbenchDetailModal({
    children,
    wrapClassName,
    style,
    styles,
    width = 'min(1040px, calc(100vw - 16px))',
    contentMinWidth = 960,
    maxBodyHeight = 'calc(100vh - 180px)',
    topOffset = 16,
    bodyPaddingRight = 8,
    canvasClassName,
    ...modalProps
}: WorkbenchDetailModalProps) {
    const mergedWrapClassName = ['workbench-detail-modal', wrapClassName]
        .filter(Boolean)
        .join(' ');
    const mergedStyle = {
        top: topOffset,
        ...style,
    };
    const mergedStyles = {
        ...styles,
        body: {
            paddingRight: bodyPaddingRight,
            ...(styles?.body ?? {}),
        },
    };

    return (
        <Modal
            {...modalProps}
            width={width}
            style={mergedStyle}
            styles={mergedStyles}
            wrapClassName={mergedWrapClassName}
        >
            <div
                className="workbench-detail-modal__viewport"
                style={{ maxHeight: maxBodyHeight }}
            >
                <div
                    className={[
                        'workbench-detail-modal__canvas',
                        canvasClassName,
                    ]
                        .filter(Boolean)
                        .join(' ')}
                    style={{ minWidth: formatContentMinWidth(contentMinWidth) }}
                >
                    {children}
                </div>
            </div>
        </Modal>
    );
}
