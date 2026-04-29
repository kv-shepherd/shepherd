'use client';

import React, { useState } from 'react';
import { createCache, extractStyle, StyleProvider } from '@ant-design/cssinjs';
import { useServerInsertedHTML } from 'next/navigation';

export default function AntdNonceRegistry({
  children,
  nonce,
}: {
  children: React.ReactNode;
  nonce?: string;
}) {
  const [cache] = useState(() => createCache());

  useServerInsertedHTML(() => {
    const styleText = extractStyle(cache, {
      plain: true,
      once: true,
    });
    if (styleText.includes('.data-ant-cssinjs-cache-path{content:"";}')) {
      return null;
    }
    return (
      <style
        id="antd-cssinjs"
        nonce={nonce}
        data-rc-order="prepend"
        data-rc-priority="-1000"
        dangerouslySetInnerHTML={{ __html: styleText }}
      />
    );
  });

  // cssinjs runtime supports `nonce`, but the published type has not caught up yet.
  const styleProviderProps = {
    cache,
    nonce,
  } as React.ComponentProps<typeof StyleProvider> & { nonce?: string };

  return (
    <StyleProvider {...styleProviderProps}>
      {children}
    </StyleProvider>
  );
}
