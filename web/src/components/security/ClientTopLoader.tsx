'use client';

import dynamic from 'next/dynamic';

const NextTopLoader = dynamic(() => import('nextjs-toploader'), {
  ssr: false,
});

export default function ClientTopLoader({ nonce }: { nonce?: string }) {
  return <NextTopLoader showSpinner={false} color="#2563eb" nonce={nonce} />;
}
