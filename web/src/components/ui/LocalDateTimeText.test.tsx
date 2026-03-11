import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { LocalDateTimeText } from './LocalDateTimeText';

describe('LocalDateTimeText', () => {
    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('renders local time in 24-hour format without inline timezone noise', () => {
        vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(function DateTimeFormat() {
            return {
            formatToParts: () => [
                { type: 'year', value: '2026' },
                { type: 'month', value: '03' },
                { type: 'day', value: '11' },
                { type: 'hour', value: '13' },
                { type: 'minute', value: '16' },
            ],
            } as Intl.DateTimeFormat;
        } as typeof Intl.DateTimeFormat);

        render(<LocalDateTimeText value="2026-03-11T05:16:00Z" />);

        expect(screen.getByText('2026-03-11 13:16')).toBeInTheDocument();
    });
});
