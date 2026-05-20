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

function parseBooleanEnv(value: string | undefined): boolean | undefined {
  if (value === undefined) {
    return undefined;
  }

  switch (value.trim().toLowerCase()) {
    case "1":
    case "true":
    case "yes":
    case "on":
      return true;
    case "0":
    case "false":
    case "no":
    case "off":
      return false;
    default:
      return undefined;
  }
}

export function shouldSendHTTPSOnlyHeaders(): boolean {
  const isProduction = process.env.NODE_ENV === "production";
  if (!isProduction) {
    return false;
  }

  const explicit = parseBooleanEnv(process.env.SHEPHERD_ENABLE_HSTS);
  if (explicit !== undefined) {
    return explicit;
  }

  const rawPublicBaseURL = (
    process.env.SHEPHERD_PUBLIC_BASE_URL ||
    process.env.SERVER_PUBLIC_BASE_URL ||
    ""
  ).trim();
  if (!rawPublicBaseURL) {
    return true;
  }

  try {
    return new URL(rawPublicBaseURL).protocol === "https:";
  } catch {
    return true;
  }
}

function resolveConfiguredPublicURL(protocol?: "http" | "https"): URL | undefined {
  const rawPublicBaseURL = (
    process.env.SHEPHERD_PUBLIC_BASE_URL ||
    process.env.SERVER_PUBLIC_BASE_URL ||
    ""
  ).trim();
  if (!rawPublicBaseURL) {
    return undefined;
  }

  try {
    const publicURL = new URL(rawPublicBaseURL);
    if (protocol && publicURL.protocol !== `${protocol}:`) {
      return undefined;
    }
    return publicURL;
  } catch {
    return undefined;
  }
}

function buildContentSecurityPolicy(nonce: string): string {
  const isProduction = process.env.NODE_ENV === "production";
  const sendHTTPSOnlyHeaders = shouldSendHTTPSOnlyHeaders();
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

  if (sendHTTPSOnlyHeaders) {
    directives.push("upgrade-insecure-requests");
  }

  return directives.join("; ");
}

function applyCspToResponse(response: NextResponse, nonce: string): NextResponse {
  response.headers.set("Content-Security-Policy", buildContentSecurityPolicy(nonce));
  if (shouldSendHTTPSOnlyHeaders()) {
    response.headers.set("Strict-Transport-Security", "max-age=31536000; includeSubDomains");
  }
  return response;
}

function firstForwardedHeaderValue(value: string | null): string {
  return (value || "").split(",")[0]?.trim() || "";
}

function resolveOriginalProtocol(request: NextRequest): "http" | "https" | undefined {
  const forwardedProto = firstForwardedHeaderValue(request.headers.get("x-forwarded-proto"))
    .toLowerCase();
  if (forwardedProto === "http" || forwardedProto === "https") {
    return forwardedProto;
  }

  const protocol = request.nextUrl.protocol.replace(":", "").toLowerCase();
  if (protocol === "http" || protocol === "https") {
    return protocol;
  }
  return undefined;
}

function resolveHTTPSPublicHost(): string | undefined {
  return resolveConfiguredPublicURL("https")?.host.toLowerCase();
}

function shouldRedirectHTTPToHTTPS(request: NextRequest, originalProtocol: string | undefined): boolean {
  if (!shouldSendHTTPSOnlyHeaders() || originalProtocol !== "http") {
    return false;
  }

  const publicHost = resolveHTTPSPublicHost();
  if (!publicHost) {
    return false;
  }

  const forwardedHost = firstForwardedHeaderValue(request.headers.get("x-forwarded-host")).toLowerCase();
  const host = (
    firstForwardedHeaderValue(request.headers.get("host")) || request.nextUrl.host
  ).toLowerCase();
  return forwardedHost === publicHost || host === publicHost;
}

function buildExternalURL(
  request: NextRequest,
  protocol: "http" | "https",
  pathname?: string,
): URL {
  const publicURL = resolveConfiguredPublicURL(protocol);
  const url = publicURL ? new URL(publicURL.origin) : request.nextUrl.clone();
  url.protocol = `${protocol}:`;
  url.pathname = pathname || request.nextUrl.pathname;
  url.search = pathname ? "" : request.nextUrl.search;

  if (!publicURL) {
    const forwardedHost = firstForwardedHeaderValue(request.headers.get("x-forwarded-host"));
    const host = forwardedHost || firstForwardedHeaderValue(request.headers.get("host"));
    if (host) {
      url.host = host;
    }
  }
  return url;
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
  const originalProtocol = resolveOriginalProtocol(request);

  if (shouldRedirectHTTPToHTTPS(request, originalProtocol)) {
    return applyCspToResponse(NextResponse.redirect(buildExternalURL(request, "https")), nonce);
  }

  if (isProtectedPath(pathname) && !hasSession) {
    return applyCspToResponse(
      NextResponse.redirect(buildExternalURL(request, originalProtocol || "https", loginPath)),
      nonce,
    );
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
