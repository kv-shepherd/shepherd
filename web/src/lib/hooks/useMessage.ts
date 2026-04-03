/**
 * useMessage — thin wrapper around Ant Design's App.useApp() message API.
 *
 * Provides a stable `messageApi` for pages already wrapped by the root
 * Ant Design <App>. `messageContextHolder` remains for compatibility and
 * is always null.
 *
 * Usage:
 *   const { messageApi, messageContextHolder } = useMessage();
 *   // Render {messageContextHolder} at the top of the component JSX.
 *   await messageApi.success('Done!');
 */
'use client';

import { App } from 'antd';

export function useMessage() {
    const { message: messageApi } = App.useApp();
    return {
        messageApi,
        messageContextHolder: null,
    };
}
