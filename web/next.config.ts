import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Transpile antd and pro-components for proper SSR/CSR handling
  transpilePackages: ["antd", "@ant-design/pro-components"],

  // Optimize barrel file imports (AGENTS.md §2.1)
  // Note: Turbopack does this automatically, but we configure it
  // explicitly for webpack fallback compatibility.
  experimental: {
    optimizePackageImports: [
      "antd",
      "@ant-design/icons",
      "@ant-design/pro-components",
      "zustand",
      "@tanstack/react-query",
      "react-i18next",
      "i18next",
      "zod",
    ],
  },
};

export default nextConfig;
