/**
 * useMessage — thin wrapper around Ant Design's App.useApp() message API.
 *
 * Provides a stable `messageApi` and `messageContextHolder` for use in
 * page components that are NOT wrapped in an Ant Design App context.
 *
 * Usage:
 *   const { messageApi, messageContextHolder } = useMessage();
 *   // Render {messageContextHolder} at the top of the component JSX.
 *   await messageApi.success('Done!');
 */
'use client';

import { message } from 'antd';

export function useMessage() {
    const [messageApi, contextHolder] = message.useMessage();
    return {
        messageApi,
        messageContextHolder: contextHolder,
    };
}
