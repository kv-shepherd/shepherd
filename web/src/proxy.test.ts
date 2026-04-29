import { afterEach, describe, expect, it } from "vitest";
import { NextRequest } from "next/server";

import { proxy } from "./proxy";

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

  it("redirects authenticated users away from the login page", () => {
    const response = proxy(
      makeRequest("https://shepherd.example.com/login", "shepherd_session=session-token"),
    );

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe("https://shepherd.example.com/dashboard");
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

    const loginResponse = proxy(
      makeRequest("https://shepherd.example.com/signin", "custom_session=session-token"),
    );
    expect(loginResponse.headers.get("location")).toBe("https://shepherd.example.com/dashboard");
  });
});
