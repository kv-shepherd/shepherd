import type { Metadata } from 'next';
import { AntdRegistry } from '@ant-design/nextjs-registry';
import NextTopLoader from 'nextjs-toploader';
import './globals.css';
import Providers from './providers';

export const metadata: Metadata = {
  title: 'KubeVirt Shepherd',
  description:
    'Cloud-native virtual machine governance platform for KubeVirt',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <NextTopLoader showSpinner={false} color="#1677ff" />
        <AntdRegistry>
          <Providers>{children}</Providers>
        </AntdRegistry>
      </body>
    </html>
  );
}
