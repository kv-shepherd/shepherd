import type { Metadata } from 'next';
import { headers } from 'next/headers';
import './globals.css';
import AntdNonceRegistry from '../components/security/AntdNonceRegistry';
import ClientTopLoader from '../components/security/ClientTopLoader';
import Providers from './providers';

export const metadata: Metadata = {
  title: 'KubeVirt Shepherd',
  description:
    'Cloud-native virtual machine governance platform for KubeVirt',
  icons: {
    icon: '/logo-icon.svg',
  },
};

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const nonce = (await headers()).get('x-nonce') ?? undefined;

  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <ClientTopLoader nonce={nonce} />
        <AntdNonceRegistry nonce={nonce}>
          <Providers nonce={nonce}>{children}</Providers>
        </AntdNonceRegistry>
      </body>
    </html>
  );
}
