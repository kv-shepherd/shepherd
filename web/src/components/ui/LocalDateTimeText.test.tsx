import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { LocalDateTimeText } from './LocalDateTimeText';

vi.mock('@/components/providers/DisplayTimeZoneProvider', () => ({
    useDisplayTimeZone: () => ({
        browserTimeZone: 'Asia/Hong_Kong',
        preferenceTimeZone: 'Asia/Hong_Kong',
        displayTimeZone: 'Asia/Hong_Kong',
        isLoading: false,
        isSaving: false,
        setTimeZone: vi.fn(),
    }),
}));

describe('LocalDateTimeText', () => {
    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('renders local time in 24-hour format without inline timezone noise', () => {
        const dateTimeFormatSpy = vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(
            function DateTimeFormat(_locales, options) {
                expect(options).toMatchObject({ timeZone: 'Asia/Hong_Kong' });

                return {
                    formatToParts: () => [
                        { type: 'year', value: '2026' },
                        { type: 'month', value: '03' },
                        { type: 'day', value: '11' },
                        { type: 'hour', value: '13' },
                        { type: 'minute', value: '16' },
                    ],
                } as Intl.DateTimeFormat;
            } as typeof Intl.DateTimeFormat
        );

        render(<LocalDateTimeText value="2026-03-11T05:16:00Z" />);

        expect(screen.getByText('2026-03-11 13:16')).toBeInTheDocument();
        expect(dateTimeFormatSpy).toHaveBeenCalled();
    });
});
