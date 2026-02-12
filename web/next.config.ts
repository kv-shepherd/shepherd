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

  // Proxy API requests to backend server (solves CORS & remote access issues)
  // When accessing from 10.x.x.x:3000, requests to /api/v1 go to localhost:8080
  async rewrites() {
    // In Docker, this should be "http://server:8080". Locally, "http://localhost:8080".
    const apiUrl = process.env.INTERNAL_API_URL || "http://localhost:8080";

    return [
      {
        source: "/api/v1/:path*",
        destination: `${apiUrl}/api/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
