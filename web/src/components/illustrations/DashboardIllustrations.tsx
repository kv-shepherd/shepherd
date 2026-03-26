'use client';

import type { SVGProps } from 'react';

type IllustrationProps = SVGProps<SVGSVGElement>;

function IllustrationFrame({ children, ...props }: IllustrationProps) {
    return (
        <svg
            viewBox="0 0 120 120"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            aria-hidden="true"
            {...props}
        >
            {children}
        </svg>
    );
}

export function SystemsOverviewGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="18" y="22" width="84" height="74" rx="22" fill="#E7F0FF" />
            <rect x="28" y="34" width="64" height="16" rx="8" fill="#BCD3FF" />
            <rect x="28" y="58" width="28" height="24" rx="12" fill="#5B8FF9" />
            <rect x="64" y="58" width="28" height="24" rx="12" fill="#8EB8FF" />
            <path d="M42 58V50M78 58V50" stroke="#2B5FD9" strokeWidth="4" strokeLinecap="round" />
            <circle cx="42" cy="70" r="5" fill="#FFFFFF" />
            <circle cx="78" cy="70" r="5" fill="#FFFFFF" />
        </IllustrationFrame>
    );
}

export function VirtualMachinesOverviewGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="18" y="22" width="84" height="74" rx="22" fill="#F1E9FF" />
            <rect x="28" y="30" width="64" height="42" rx="12" fill="#6D4DE3" />
            <rect x="34" y="36" width="52" height="30" rx="10" fill="#FFFFFF" fillOpacity="0.16" />
            <path d="M47 84H73" stroke="#6D4DE3" strokeWidth="6" strokeLinecap="round" />
            <path d="M60 72V84" stroke="#6D4DE3" strokeWidth="6" strokeLinecap="round" />
            <path d="M48 51H72" stroke="#FFFFFF" strokeWidth="6" strokeLinecap="round" />
            <path d="M48 43H64" stroke="#CFC1FF" strokeWidth="6" strokeLinecap="round" />
        </IllustrationFrame>
    );
}

export function RequestsOverviewGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="18" y="22" width="84" height="74" rx="22" fill="#FFF0E6" />
            <rect x="34" y="28" width="52" height="64" rx="14" fill="#FFFFFF" />
            <rect x="45" y="22" width="30" height="14" rx="7" fill="#FFB57A" />
            <path d="M45 50H75" stroke="#D66A1F" strokeWidth="5" strokeLinecap="round" />
            <path d="M45 64H69" stroke="#FF9A4A" strokeWidth="5" strokeLinecap="round" />
            <path d="M45 78H63" stroke="#FFD0A6" strokeWidth="5" strokeLinecap="round" />
            <circle cx="79" cy="74" r="13" fill="#D66A1F" />
            <path d="M74 74L78 78L85 69" stroke="#FFFFFF" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" />
        </IllustrationFrame>
    );
}

export function HealthOverviewGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="16" y="24" width="88" height="72" rx="24" fill="#E8FFF2" />
            <rect x="30" y="36" width="60" height="48" rx="16" fill="#0F8F57" fillOpacity="0.12" />
            <path d="M38 60H50L56 50L64 70L70 60H82" stroke="#0F8F57" strokeWidth="5" strokeLinecap="round" strokeLinejoin="round" />
            <circle cx="44" cy="46" r="4" fill="#12B76A" />
            <circle cx="76" cy="46" r="4" fill="#12B76A" />
            <path d="M52 30L60 24L68 30V42C68 50 63 56 60 58C57 56 52 50 52 42V30Z" fill="#12B76A" />
            <path d="M57 41L59 43L64 37" stroke="#FFFFFF" strokeWidth="3.5" strokeLinecap="round" strokeLinejoin="round" />
        </IllustrationFrame>
    );
}

export function DraftNotebookGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="18" y="22" width="84" height="74" rx="22" fill="#E8F1FF" />
            <rect x="32" y="28" width="52" height="64" rx="14" fill="#FFFFFF" />
            <rect x="40" y="22" width="36" height="14" rx="7" fill="#8EB8FF" />
            <path d="M44 48H72" stroke="#2B6BFF" strokeWidth="5" strokeLinecap="round" />
            <path d="M44 62H72" stroke="#6F9BFF" strokeWidth="5" strokeLinecap="round" />
            <path d="M44 76H62" stroke="#BCD3FF" strokeWidth="5" strokeLinecap="round" />
            <circle cx="82" cy="78" r="12" fill="#2B6BFF" />
            <path d="M76 78L80 82L88 72" stroke="#FFFFFF" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" />
        </IllustrationFrame>
    );
}

export function QueueReviewGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="18" y="22" width="84" height="74" rx="22" fill="#FFF6E8" />
            <rect x="28" y="34" width="40" height="14" rx="7" fill="#FFD39A" />
            <rect x="28" y="56" width="50" height="14" rx="7" fill="#FFB964" />
            <rect x="28" y="78" width="36" height="10" rx="5" fill="#FFE7C2" />
            <circle cx="82" cy="58" r="17" fill="#D66A1F" />
            <path d="M82 49V58L88 62" stroke="#FFFFFF" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" />
            <circle cx="82" cy="84" r="10" fill="#FFF0D9" stroke="#D66A1F" strokeWidth="4" />
            <path d="M77 84L80 87L87 79" stroke="#D66A1F" strokeWidth="3.5" strokeLinecap="round" strokeLinejoin="round" />
        </IllustrationFrame>
    );
}

export function BatchFlowGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="16" y="20" width="88" height="80" rx="24" fill="#FFF3E8" />
            <rect x="24" y="32" width="28" height="18" rx="9" fill="#FFB57A" />
            <rect x="68" y="32" width="28" height="18" rx="9" fill="#FFD7AE" />
            <rect x="46" y="70" width="28" height="18" rx="9" fill="#D66A1F" />
            <path d="M38 50V58C38 64 42 68 48 68H60" stroke="#D66A1F" strokeWidth="4.5" strokeLinecap="round" />
            <path d="M82 50V58C82 64 78 68 72 68H60" stroke="#D66A1F" strokeWidth="4.5" strokeLinecap="round" />
            <circle cx="60" cy="68" r="6" fill="#FFFFFF" stroke="#D66A1F" strokeWidth="4" />
        </IllustrationFrame>
    );
}

export function ServiceWorkspaceGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="18" y="22" width="84" height="74" rx="22" fill="#EAFBF6" />
            <rect x="28" y="32" width="34" height="26" rx="10" fill="#40C79F" />
            <rect x="58" y="28" width="34" height="34" rx="12" fill="#12B76A" fillOpacity="0.18" />
            <rect x="34" y="40" width="22" height="10" rx="5" fill="#FFFFFF" fillOpacity="0.9" />
            <path d="M45 58V72" stroke="#12B76A" strokeWidth="4.5" strokeLinecap="round" />
            <path d="M45 72H79" stroke="#12B76A" strokeWidth="4.5" strokeLinecap="round" />
            <circle cx="79" cy="72" r="10" fill="#12B76A" />
            <path d="M75 72L78 75L84 68" stroke="#FFFFFF" strokeWidth="3.5" strokeLinecap="round" strokeLinejoin="round" />
        </IllustrationFrame>
    );
}

export function NotificationInboxGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="16" y="22" width="88" height="76" rx="24" fill="#EEF4FF" />
            <path d="M30 40H90V82C90 88.627 84.627 94 78 94H42C35.373 94 30 88.627 30 82V40Z" fill="#FFFFFF" />
            <path d="M30 46L60 66L90 46" stroke="#2B6BFF" strokeWidth="5" strokeLinecap="round" strokeLinejoin="round" />
            <path d="M30 82L50 64" stroke="#B7CCFF" strokeWidth="4.5" strokeLinecap="round" />
            <path d="M90 82L70 64" stroke="#B7CCFF" strokeWidth="4.5" strokeLinecap="round" />
            <circle cx="84" cy="36" r="10" fill="#FF6B6B" />
            <path d="M80 36H88" stroke="#FFFFFF" strokeWidth="3.5" strokeLinecap="round" />
            <path d="M84 32V40" stroke="#FFFFFF" strokeWidth="3.5" strokeLinecap="round" />
        </IllustrationFrame>
    );
}

export function TemplateCatalogGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="16" y="20" width="88" height="80" rx="24" fill="#FFF5EA" />
            <rect x="28" y="30" width="52" height="62" rx="14" fill="#FFFFFF" />
            <rect x="40" y="24" width="28" height="14" rx="7" fill="#FFB964" />
            <path d="M42 50H66" stroke="#D66A1F" strokeWidth="5" strokeLinecap="round" />
            <path d="M42 64H72" stroke="#FF9A4A" strokeWidth="5" strokeLinecap="round" />
            <path d="M42 78H60" stroke="#FFD6B0" strokeWidth="5" strokeLinecap="round" />
            <rect x="72" y="42" width="20" height="20" rx="8" fill="#FFF0D9" stroke="#D66A1F" strokeWidth="4" />
            <path d="M82 48V56" stroke="#D66A1F" strokeWidth="3.5" strokeLinecap="round" />
            <path d="M78 52H86" stroke="#D66A1F" strokeWidth="3.5" strokeLinecap="round" />
        </IllustrationFrame>
    );
}

export function InstanceSizeBlueprintGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="16" y="20" width="88" height="80" rx="24" fill="#EEF8FF" />
            <rect x="26" y="30" width="68" height="60" rx="18" fill="#FFFFFF" />
            <rect x="38" y="42" width="20" height="20" rx="8" fill="#B5DBFF" />
            <rect x="62" y="42" width="20" height="20" rx="8" fill="#5BA7FF" />
            <path d="M38 72H82" stroke="#2B6BFF" strokeWidth="5" strokeLinecap="round" />
            <path d="M48 36V30M72 36V30M48 90V84M72 90V84" stroke="#2B6BFF" strokeWidth="4" strokeLinecap="round" />
            <path d="M30 52H26M94 52H90" stroke="#2B6BFF" strokeWidth="4" strokeLinecap="round" />
            <circle cx="72" cy="74" r="10" fill="#2B6BFF" />
            <path d="M67 74L70 77L77 70" stroke="#FFFFFF" strokeWidth="3.5" strokeLinecap="round" strokeLinejoin="round" />
        </IllustrationFrame>
    );
}

export function DecisionRejectedGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="18" y="22" width="84" height="74" rx="22" fill="#FFF1F0" />
            <rect x="34" y="28" width="52" height="64" rx="14" fill="#FFFFFF" />
            <rect x="45" y="22" width="30" height="14" rx="7" fill="#FF9B9B" />
            <path d="M45 50H75" stroke="#E5484D" strokeWidth="5" strokeLinecap="round" />
            <path d="M45 64H69" stroke="#FFB3B6" strokeWidth="5" strokeLinecap="round" />
            <circle cx="79" cy="74" r="13" fill="#E5484D" />
            <path d="M74 69L84 79" stroke="#FFFFFF" strokeWidth="4" strokeLinecap="round" />
            <path d="M84 69L74 79" stroke="#FFFFFF" strokeWidth="4" strokeLinecap="round" />
        </IllustrationFrame>
    );
}

export function UserDirectoryGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="16" y="20" width="88" height="80" rx="24" fill="#EDF6FF" />
            <circle cx="46" cy="48" r="12" fill="#5B8FF9" />
            <path d="M30 82C30 70.954 38.954 62 50 62H58C69.046 62 78 70.954 78 82" fill="#CFE1FF" />
            <circle cx="78" cy="44" r="10" fill="#B6D2FF" />
            <path d="M66 80C66 71.716 72.716 65 81 65H85C93.284 65 100 71.716 100 80" fill="#E1EDFF" />
        </IllustrationFrame>
    );
}

export function AccessControlGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="18" y="22" width="84" height="74" rx="22" fill="#EEF7F4" />
            <path d="M60 28L84 36V54C84 70 72.8 82 60 88C47.2 82 36 70 36 54V36L60 28Z" fill="#12B76A" fillOpacity="0.18" stroke="#0F8F57" strokeWidth="4" />
            <path d="M50 58L57 65L72 48" stroke="#0F8F57" strokeWidth="5" strokeLinecap="round" strokeLinejoin="round" />
            <rect x="28" y="36" width="18" height="12" rx="6" fill="#CFF5E3" />
            <rect x="74" y="36" width="18" height="12" rx="6" fill="#CFF5E3" />
        </IllustrationFrame>
    );
}

export function RoleCatalogGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="18" y="22" width="84" height="74" rx="22" fill="#FFF5EA" />
            <rect x="30" y="32" width="60" height="50" rx="14" fill="#FFFFFF" />
            <rect x="40" y="24" width="40" height="16" rx="8" fill="#FFB964" />
            <path d="M42 52H78" stroke="#D66A1F" strokeWidth="5" strokeLinecap="round" />
            <path d="M42 66H70" stroke="#FF9A4A" strokeWidth="5" strokeLinecap="round" />
            <circle cx="79" cy="72" r="11" fill="#D66A1F" />
            <path d="M79 67V77" stroke="#FFFFFF" strokeWidth="3.5" strokeLinecap="round" />
            <path d="M74 72H84" stroke="#FFFFFF" strokeWidth="3.5" strokeLinecap="round" />
        </IllustrationFrame>
    );
}

export function RateLimitGaugeGlyph(props: IllustrationProps) {
    return (
        <IllustrationFrame {...props}>
            <rect x="16" y="20" width="88" height="80" rx="24" fill="#F4F0FF" />
            <path d="M34 76C34 61.641 45.641 50 60 50C74.359 50 86 61.641 86 76" stroke="#D7CAFF" strokeWidth="10" strokeLinecap="round" />
            <path d="M34 76C34 61.641 45.641 50 60 50" stroke="#6D4DE3" strokeWidth="10" strokeLinecap="round" />
            <path d="M60 76L72 58" stroke="#6D4DE3" strokeWidth="5" strokeLinecap="round" />
            <circle cx="60" cy="76" r="7" fill="#6D4DE3" />
            <rect x="28" y="34" width="20" height="10" rx="5" fill="#CFC1FF" />
            <rect x="72" y="34" width="20" height="10" rx="5" fill="#E5DDFF" />
        </IllustrationFrame>
    );
}
