import { NextRequest, NextResponse } from "next/server";

const defaultLoginPath = "/login";
const defaultSessionCookieName = "shepherd_session";
const protectedRoutePrefixes = [
  "/admin",
  "/approvals",
  "/auth/change-password",
  "/dashboard",
  "/notifications",
  "/profile",
  "/services",
  "/systems",
  "/tickets",
  "/vms",
];
const forwardedRequestHeaderAllowList = new Set([
  "accept",
  "accept-language",
  "cache-control",
  "next-action",
  "next-router-prefetch",
  "next-router-state-tree",
  "next-url",
  "pragma",
  "purpose",
  "referer",
  "rsc",
  "sec-fetch-dest",
  "sec-fetch-mode",
  "sec-fetch-site",
  "sec-fetch-user",
  "user-agent",
]);

function buildContentSecurityPolicy(nonce: string): string {
  const isProduction = process.env.NODE_ENV === "production";
  const scriptSources = ["'self'", `'nonce-${nonce}'`, "'strict-dynamic'"];
  const styleSources = ["'self'", isProduction ? `'nonce-${nonce}'` : "'unsafe-inline'"];
  const connectSources = ["'self'"];
  if (!isProduction) {
    scriptSources.push("'unsafe-eval'");
    connectSources.push("ws:", "wss:");
  }

  const directives = [
    "default-src 'self'",
    `script-src ${scriptSources.join(" ")}`,
    `style-src ${styleSources.join(" ")}`,
    "img-src 'self' data: blob:",
    "font-src 'self' data:",
    `connect-src ${connectSources.join(" ")}`,
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ];

  if (isProduction) {
    directives.push("upgrade-insecure-requests");
  }

  return directives.join("; ");
}

function applyCspToResponse(response: NextResponse, nonce: string): NextResponse {
  response.headers.set("Content-Security-Policy", buildContentSecurityPolicy(nonce));
  return response;
}

export function buildForwardedRequestHeaders(headers: Headers, nonce: string): Headers {
  const forwarded = new Headers();
  for (const [name, value] of headers) {
    const normalized = name.toLowerCase();
    if (forwardedRequestHeaderAllowList.has(normalized)) {
      forwarded.set(name, value);
    }
  }
  forwarded.set("x-nonce", nonce);
  forwarded.set("Content-Security-Policy", buildContentSecurityPolicy(nonce));
  return forwarded;
}

function normalizePath(raw: string | undefined, fallback: string): string {
  const candidate = (raw || "").trim();
  if (!candidate.startsWith("/") || candidate.startsWith("//")) {
    return fallback;
  }
  return candidate;
}

function resolveLoginPath(): string {
  return normalizePath(process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH, defaultLoginPath);
}

function normalizeCookieName(raw: string | undefined, fallback: string): string {
  const candidate = (raw || "").trim();
  return candidate || fallback;
}

function resolveSessionCookieName(): string {
  return normalizeCookieName(
    process.env.SESSION_COOKIE || process.env.NEXT_PUBLIC_SESSION_COOKIE_NAME,
    defaultSessionCookieName,
  );
}

function isProtectedPath(pathname: string): boolean {
  return protectedRoutePrefixes.some((prefix) =>
    pathname === prefix || pathname.startsWith(`${prefix}/`),
  );
}

export function proxy(request: NextRequest) {
  const nonce = Buffer.from(crypto.randomUUID()).toString("base64");
  const loginPath = resolveLoginPath();
  const pathname = request.nextUrl.pathname;
  const hasSession = request.cookies.has(resolveSessionCookieName());

  if (isProtectedPath(pathname) && !hasSession) {
    return applyCspToResponse(NextResponse.redirect(new URL(loginPath, request.url)), nonce);
  }

  const requestHeaders = buildForwardedRequestHeaders(request.headers, nonce);

  const response = NextResponse.next({
    request: {
      headers: requestHeaders,
    },
  });
  return applyCspToResponse(response, nonce);
}

export const config = {
  matcher: [
    {
      source: "/((?!api|_next/static|_next/image|favicon.ico).*)",
      missing: [
        { type: "header", key: "next-router-prefetch" },
        { type: "header", key: "purpose", value: "prefetch" },
      ],
    },
  ],
};
