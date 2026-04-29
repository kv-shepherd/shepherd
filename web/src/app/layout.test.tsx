import React from 'react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('next/headers', () => ({
  headers: vi.fn(async () => new Headers()),
}));

vi.mock('../components/security/AntdNonceRegistry', () => ({
  default: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock('../components/security/ClientTopLoader', () => ({
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
