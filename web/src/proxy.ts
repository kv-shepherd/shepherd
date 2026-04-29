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
  const loginPath = resolveLoginPath();
  const pathname = request.nextUrl.pathname;
  const hasSession = request.cookies.has(resolveSessionCookieName());

  if (isProtectedPath(pathname) && !hasSession) {
    return NextResponse.redirect(new URL(loginPath, request.url));
  }

  if (pathname === loginPath && hasSession) {
    return NextResponse.redirect(new URL("/dashboard", request.url));
  }

  return NextResponse.next();
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
