'use client';

import React from 'react';

import { useUserPreference } from '@/hooks/useUserPreference';
import { getBrowserTimeZone } from '@/lib/timeZone';

const DISPLAY_TIME_ZONE_PREFERENCE_KEY = 'display_timezone';

interface DisplayTimeZonePreferenceValue {
    timeZone?: string | null;
}

interface DisplayTimeZoneContextValue {
    browserTimeZone: string | null;
    preferenceTimeZone: string | null;
    displayTimeZone: string | null;
    isLoading: boolean;
    isSaving: boolean;
    setTimeZone: (timeZone: string | null) => Promise<void>;
}

const defaultContextValue: DisplayTimeZoneContextValue = {
    browserTimeZone: null,
    preferenceTimeZone: null,
    displayTimeZone: null,
    isLoading: false,
    isSaving: false,
    setTimeZone: async () => {},
};

const DisplayTimeZoneContext = React.createContext<DisplayTimeZoneContextValue>(defaultContextValue);

export function DisplayTimeZoneProvider({
    children,
}: {
    children: React.ReactNode;
}) {
    const preference = useUserPreference<DisplayTimeZonePreferenceValue>(DISPLAY_TIME_ZONE_PREFERENCE_KEY);
    const browserTimeZone = React.useMemo(() => getBrowserTimeZone(), []);
    const preferenceTimeZone = normalizeTimeZone(preference.value?.timeZone);

    const setTimeZone = React.useCallback(
        async (timeZone: string | null) => {
            const normalizedTimeZone = normalizeTimeZone(timeZone);
            if (!normalizedTimeZone) {
                await preference.resetPreference();
                return;
            }

            await preference.savePreference({
                value: {
                    timeZone: normalizedTimeZone,
                },
            });
        },
        [preference]
    );

    const contextValue = React.useMemo(
        () => ({
            browserTimeZone,
            preferenceTimeZone,
            displayTimeZone: preferenceTimeZone ?? browserTimeZone,
            isLoading: preference.isLoading,
            isSaving: preference.savePending || preference.resetPending,
            setTimeZone,
        }),
        [
            browserTimeZone,
            preference.isLoading,
            preference.resetPending,
            preference.savePending,
            preferenceTimeZone,
            setTimeZone,
        ]
    );

    return (
        <DisplayTimeZoneContext.Provider value={contextValue}>
            {children}
        </DisplayTimeZoneContext.Provider>
    );
}

export function useDisplayTimeZone() {
    return React.useContext(DisplayTimeZoneContext);
}

function normalizeTimeZone(value?: string | null): string | null {
    if (!value) {
        return null;
    }

    const trimmedValue = value.trim();
    return trimmedValue === '' ? null : trimmedValue;
}
