import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import LocalTimezoneBadge from './LocalTimezoneBadge';

const mockTimeZoneState = vi.hoisted(() => ({
    browserTimeZone: 'Asia/Hong_Kong',
    preferenceTimeZone: null as string | null,
}));

vi.mock('@/components/providers/DisplayTimeZoneProvider', () => ({
    useDisplayTimeZone: () => ({
        browserTimeZone: mockTimeZoneState.browserTimeZone,
        preferenceTimeZone: mockTimeZoneState.preferenceTimeZone,
        displayTimeZone: mockTimeZoneState.preferenceTimeZone ?? mockTimeZoneState.browserTimeZone,
        isLoading: false,
        isSaving: false,
        setTimeZone: vi.fn(),
    }),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string; timeZone?: string }) => {
            if (key === 'status.local_timezone_follow_browser') {
                return `Follow browser · ${options?.timeZone}`;
            }
            if (key === 'status.local_timezone_auto') {
                return 'Automatic';
            }
            if (key === 'status.display_timezone') {
                return 'Display timezone';
            }
            if (key === 'status.timezone.city.Asia_Hong_Kong') {
                return 'Hong Kong';
            }
            if (key === 'status.timezone.city.Asia_Shanghai') {
                return 'Beijing';
            }
            if (key.startsWith('status.timezone.city.')) {
                return options?.defaultValue ?? key;
            }
            return key;
        },
    }),
}));

describe('LocalTimezoneBadge', () => {
    afterEach(() => {
        mockTimeZoneState.browserTimeZone = 'Asia/Hong_Kong';
        mockTimeZoneState.preferenceTimeZone = null;
        vi.restoreAllMocks();
    });

    it('renders the display timezone selector with browser fallback selected', () => {
        render(<LocalTimezoneBadge />);

        expect(screen.getByRole('combobox', { name: 'Display timezone' })).toBeInTheDocument();
        expect(screen.getByText('UTC+8 · Hong Kong')).toBeInTheDocument();
    });

    it('localizes the Asia/Shanghai display label as Beijing', () => {
        mockTimeZoneState.preferenceTimeZone = 'Asia/Shanghai';

        render(<LocalTimezoneBadge />);

        expect(screen.getByText('UTC+8 · Beijing')).toBeInTheDocument();
    });
});
