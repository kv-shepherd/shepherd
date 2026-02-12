/**
 * Global Loading UI (AGENTS.md §8.2: Handle async route transitions).
 *
 * Automatically shown by Next.js while a route segment is being fetched/rendered.
 * Prevents the "frozen" feeling during navigation.
 */
import { Spin } from 'antd';

export default function Loading() {
    return (
        <div
            style={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                height: '100vh',
                width: '100%',
            }}
        >
            <Spin size="large" fullscreen tip="Loading..." />
        </div>
    );
}
