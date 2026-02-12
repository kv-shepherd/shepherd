'use client';

/**
 * Global Template (AGENTS.md §8.3: Smooth transitions).
 *
 * Wraps page content to provide a smooth fade-in animation slightly
 * distinct from Layout (which persists). This helps prevent the
 * "sudden switch" feeling by easing in the new content.
 */
import { usePathname } from 'next/navigation';

export default function Template({ children }: { children: React.ReactNode }) {
    // Key by pathname to trigger re-render and animation on route change
    // even if the template component itself is not remounted by React's diffing (though templates usually remount).
    const pathname = usePathname();

    return (
        <div key={pathname} className="animate-fade-in">
            {children}
        </div>
    );
}
