import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

import type { NextConfig } from "next";

// ESM compatibility: Next.js 16 compiles .ts config to ESM where
// `require` and `__dirname` are not available as globals.
const esmRequire = createRequire(import.meta.url);
const __esmDirname = path.dirname(fileURLToPath(import.meta.url));

const allowedDevOrigins = (process.env.DEV_ALLOWED_ORIGINS || "")
  .split(",")
  .map((origin) => origin.trim())
  .filter(Boolean);

function resolveInternalAPIURL(): string {
  const raw = process.env.INTERNAL_API_URL || "http://localhost:8080";
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch (error) {
    throw new Error(`INTERNAL_API_URL must be an absolute http(s) URL: ${(error as Error).message}`);
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new Error("INTERNAL_API_URL must use http or https");
  }
  if (parsed.username || parsed.password) {
    throw new Error("INTERNAL_API_URL must not include credentials");
  }
  if (parsed.search || parsed.hash) {
    throw new Error("INTERNAL_API_URL must not include query string or fragment");
  }
  return parsed.toString().replace(/\/$/, "");
}

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

  turbopack: {},

  // Proxy API requests to backend server (solves CORS & remote access issues)
  // When accessing from 10.x.x.x:3000, requests to /api/v1 go to localhost:8080
  async rewrites() {
    // In Docker, this should be "http://server:8080". Locally, "http://localhost:8080".
    const apiUrl = resolveInternalAPIURL();

    return [
      {
        source: "/api/v1/:path*",
        destination: `${apiUrl}/api/v1/:path*`,
      },
    ];
  },

  async headers() {
    const securityHeaders = [
      { key: "X-Frame-Options", value: "DENY" },
      { key: "X-Content-Type-Options", value: "nosniff" },
      { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
      {
        key: "Permissions-Policy",
        value: "camera=(), microphone=(), geolocation=()",
      },
    ];
    return [
      {
        source: "/:path*",
        headers: securityHeaders,
      },
    ];
  },

  webpack: (config) => {
    config.resolve = config.resolve || {};
    config.resolve.alias = {
      ...(config.resolve.alias || {}),
      [esmRequire.resolve("@novnc/novnc/lib/util/browser.js")]: path.resolve(
        __esmDirname,
        "src/vendor/novnc/browser.ts",
      ),
    };
    return config;
  },
};

export default nextConfig;
