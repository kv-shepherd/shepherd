'use client';

import { useEffect, useRef } from 'react';

export function useAutoOpenIntent(
    expectedIntent: string,
    onMatch: (params: URLSearchParams) => void,
) {
    const handledRef = useRef(false);

    useEffect(() => {
        if (typeof window === 'undefined') {
            return;
        }

        if (handledRef.current) {
            return;
        }

        const url = new URL(window.location.href);
        if (url.searchParams.get('intent') !== expectedIntent) {
            return;
        }

        handledRef.current = true;
        onMatch(new URLSearchParams(url.searchParams));

        url.searchParams.delete('intent');
        const nextURL = `${url.pathname}${url.searchParams.toString() === '' ? '' : `?${url.searchParams.toString()}`}${url.hash}`;
        window.history.replaceState(window.history.state, '', nextURL);
    }, [expectedIntent, onMatch]);
}
