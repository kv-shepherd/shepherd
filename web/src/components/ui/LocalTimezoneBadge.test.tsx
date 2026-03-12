import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import LocalTimezoneBadge from './LocalTimezoneBadge';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { timeZone?: string }) =>
            key === 'status.local_timezone' ? `Local TZ: ${options?.timeZone}` : key,
    }),
}));

describe('LocalTimezoneBadge', () => {
    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('renders the browser local UTC offset in the header badge', () => {
        vi.spyOn(Date.prototype, 'getTimezoneOffset').mockReturnValue(-480);

        render(<LocalTimezoneBadge />);

        expect(screen.getByText('Local TZ: UTC+8')).toBeInTheDocument();
    });
});
