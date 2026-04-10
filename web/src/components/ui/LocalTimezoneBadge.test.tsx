import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import LocalTimezoneBadge from './LocalTimezoneBadge';

vi.mock('@/components/providers/DisplayTimeZoneProvider', () => ({
    useDisplayTimeZone: () => ({
        browserTimeZone: 'Asia/Hong_Kong',
        preferenceTimeZone: null,
        displayTimeZone: 'Asia/Hong_Kong',
        isLoading: false,
        isSaving: false,
        setTimeZone: vi.fn(),
    }),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { timeZone?: string }) => {
            if (key === 'status.local_timezone_follow_browser') {
                return `Follow browser · ${options?.timeZone}`;
            }
            if (key === 'status.local_timezone_auto') {
                return 'Automatic';
            }
            if (key === 'status.display_timezone') {
                return 'Display timezone';
            }
            return key;
        },
    }),
}));

describe('LocalTimezoneBadge', () => {
    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('renders the display timezone selector with browser fallback selected', () => {
        render(<LocalTimezoneBadge />);

        expect(screen.getByRole('combobox', { name: 'Display timezone' })).toBeInTheDocument();
        expect(screen.getByText('UTC+8 · Hong Kong')).toBeInTheDocument();
    });
});
