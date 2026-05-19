import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";

import { buildForwardedRequestHeaders, proxy } from "./proxy";

const originalLoginEntry = process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH;
const originalSessionCookie = process.env.SESSION_COOKIE;

function makeRequest(url: string, cookieHeader?: string) {
  return new NextRequest(url, {
    headers: cookieHeader ? { cookie: cookieHeader } : undefined,
  });
}

afterEach(() => {
  if (originalLoginEntry === undefined) {
    delete process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH;
  } else {
    process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH = originalLoginEntry;
  }

  if (originalSessionCookie === undefined) {
    delete process.env.SESSION_COOKIE;
  } else {
    process.env.SESSION_COOKIE = originalSessionCookie;
  }

  vi.unstubAllEnvs();
});

describe("proxy", () => {
  it("redirects unauthenticated users away from protected routes", () => {
    const response = proxy(makeRequest("https://shepherd.example.com/dashboard"));

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe("https://shepherd.example.com/login");
  });

  it("allows unauthenticated users to access the login page", () => {
    const response = proxy(makeRequest("https://shepherd.example.com/login"));

    expect(response.headers.get("location")).toBeNull();
  });

  it("allows the login page to validate a session cookie client-side", () => {
    const response = proxy(
      makeRequest("https://shepherd.example.com/login", "shepherd_session=session-token"),
    );

    expect(response.headers.get("location")).toBeNull();
  });

  it("treats the password change page as protected", () => {
    const response = proxy(makeRequest("https://shepherd.example.com/auth/change-password"));

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe("https://shepherd.example.com/login");
  });

  it("honors configured login and session cookie names", () => {
    process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH = "/signin";
    process.env.SESSION_COOKIE = "custom_session";

    const protectedResponse = proxy(makeRequest("https://shepherd.example.com/dashboard"));
    expect(protectedResponse.headers.get("location")).toBe("https://shepherd.example.com/signin");

    const protectedCookieResponse = proxy(
      makeRequest("https://shepherd.example.com/dashboard", "custom_session=session-token"),
    );
    expect(protectedCookieResponse.headers.get("location")).toBeNull();

    const loginResponse = proxy(
      makeRequest("https://shepherd.example.com/signin", "custom_session=session-token"),
    );
    expect(loginResponse.headers.get("location")).toBeNull();
  });

  it("emits nonce-based CSP without HTTPS-only directives for HTTP public URLs", () => {
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("SHEPHERD_PUBLIC_BASE_URL", "http://shepherd.example.com");

    const response = proxy(makeRequest("https://shepherd.example.com/dashboard", "shepherd_session=session-token"));
    const csp = response.headers.get("content-security-policy");

    expect(csp).toContain("script-src 'self' 'nonce-");
    expect(csp).toContain("'strict-dynamic'");
    expect(csp).toContain("style-src 'self' 'nonce-");
    expect(csp).not.toContain("'unsafe-inline'");
    expect(csp).not.toContain("upgrade-insecure-requests");
    expect(response.headers.get("strict-transport-security")).toBeNull();
  });

  it("emits HTTPS-only headers for HTTPS public URLs", () => {
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("SHEPHERD_PUBLIC_BASE_URL", "https://shepherd.example.com");

    const response = proxy(makeRequest("https://shepherd.example.com/dashboard", "shepherd_session=session-token"));
    const csp = response.headers.get("content-security-policy");

    expect(csp).toContain("upgrade-insecure-requests");
    expect(response.headers.get("strict-transport-security")).toBe("max-age=31536000; includeSubDomains");
  });

  it("forwards only allow-listed request headers plus CSP nonce", () => {
    const forwarded = buildForwardedRequestHeaders(
      new Headers({
        accept: "text/x-component",
        authorization: "Bearer secret",
        cookie: "shepherd_session=secret",
        "next-unreviewed-header": "unexpected",
        "next-router-state-tree": "tree",
        "x-custom-debug": "debug",
        "x-nextjs-cache": "spoofed",
      }),
      "nonce-value",
    );

    expect(forwarded.get("accept")).toBe("text/x-component");
    expect(forwarded.get("next-router-state-tree")).toBe("tree");
    expect(forwarded.get("authorization")).toBeNull();
    expect(forwarded.get("cookie")).toBeNull();
    expect(forwarded.get("next-unreviewed-header")).toBeNull();
    expect(forwarded.get("x-custom-debug")).toBeNull();
    expect(forwarded.get("x-nextjs-cache")).toBeNull();
    expect(forwarded.get("x-nonce")).toBe("nonce-value");
    expect(forwarded.get("content-security-policy")).toContain("'nonce-nonce-value'");
  });
});
