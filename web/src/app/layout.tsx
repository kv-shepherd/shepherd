import type { Metadata } from 'next';
import { AntdRegistry } from '@ant-design/nextjs-registry';
import { Geist, IBM_Plex_Mono } from 'next/font/google';
import NextTopLoader from 'nextjs-toploader';
import './globals.css';
import Providers from './providers';

const geistSans = Geist({
  subsets: ['latin'],
  variable: '--font-geist-sans',
  display: 'swap',
});

const plexMono = IBM_Plex_Mono({
  subsets: ['latin'],
  variable: '--font-ibm-plex-mono',
  weight: ['400', '500', '600'],
  display: 'swap',
});

export const metadata: Metadata = {
  title: 'KubeVirt Shepherd',
  description:
    'Cloud-native virtual machine governance platform for KubeVirt',
  icons: {
    icon: '/logo-icon.svg',
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={`${geistSans.variable} ${plexMono.variable}`}
    >
      <body>
        <NextTopLoader showSpinner={false} color="#2563eb" />
        <AntdRegistry>
          <Providers>{children}</Providers>
        </AntdRegistry>
      </body>
    </html>
  );
}
