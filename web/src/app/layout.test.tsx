import React from 'react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('next/font/google', () => ({
  Geist: () => ({ variable: '--font-geist-sans' }),
  IBM_Plex_Mono: () => ({ variable: '--font-ibm-plex-mono' }),
}));

vi.mock('@ant-design/nextjs-registry', () => ({
  AntdRegistry: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock('nextjs-toploader', () => ({
  default: () => null,
}));

vi.mock('./providers', () => ({
  default: ({ children }: { children: React.ReactNode }) => children,
}));

import { metadata } from './layout';

describe('app layout metadata', () => {
  it('uses the logo icon as the favicon source', () => {
    expect(metadata.icons).toMatchObject({
      icon: '/logo-icon.svg',
    });
  });
});
