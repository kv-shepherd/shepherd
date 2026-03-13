import type { NextConfig } from "next";

const allowedDevOrigins = (process.env.DEV_ALLOWED_ORIGINS || "")
  .split(",")
  .map((origin) => origin.trim())
  .filter(Boolean);

const nextConfig: NextConfig = {
  allowedDevOrigins,

  // Transpile antd and pro-components for proper SSR/CSR handling
  transpilePackages: ["antd", "@ant-design/pro-components"],

  // E2E isolation:
  // - production/local dev default: ".next"
  // - live E2E default: ".next-e2e"
  // - live E2E per-run override: NEXT_DIST_DIR (set by run_e2e_live.sh)
  // Per-run dist dir avoids stale/concurrent Next dev lock conflicts.
  distDir: process.env.NEXT_DIST_DIR || (process.env.LIVE_E2E ? ".next-e2e" : ".next"),

  // For live E2E runs we route Next's tsconfig auto-adjustment to a temporary file
  // (under .next-e2e) so repository tsconfig.json is not rewritten on each run.
  typescript: {
    tsconfigPath: process.env.NEXT_TSCONFIG_PATH || "tsconfig.json",
  },

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
        source: "/health/:path*",
        destination: `${apiUrl}/health/:path*`,
      },
      {
        source: "/api/v1/:path*",
        destination: `${apiUrl}/api/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
