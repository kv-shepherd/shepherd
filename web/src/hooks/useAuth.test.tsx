import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/stores/auth";

const { pushMock, messageErrorMock, tMock, postMock, getMock } = vi.hoisted(
  () => ({
    pushMock: vi.fn(),
    messageErrorMock: vi.fn(),
    tMock: vi.fn((key: string) => key),
    postMock: vi.fn(),
    getMock: vi.fn(),
  }),
);

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: pushMock,
  }),
}));

vi.mock("antd", () => ({
  App: {
    useApp: () => ({
      message: {
        error: messageErrorMock,
      },
    }),
  },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: tMock,
  }),
}));

vi.mock("@/lib/api/client", () => ({
  api: {
    POST: postMock,
    GET: getMock,
  },
}));

import { useAuth } from "./useAuth";

function resetAuthStore() {
  useAuthStore.setState({
    user: null,
    isAuthenticated: false,
    forcePasswordChange: false,
    hasHydrated: true,
    hasValidatedSession: false,
  });
}

describe("useAuth", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    resetAuthStore();
  });

  it("stores session user and redirects to dashboard on successful login", async () => {
    postMock.mockResolvedValue({
      data: {
        token: "token-1",
        force_password_change: false,
      },
    });
    getMock.mockResolvedValue({
      data: {
        id: "u-1",
        username: "alice",
      },
    });

    const { result } = renderHook(() => useAuth());
    await act(async () => {
      await result.current.login({ username: "alice", password: "secret" });
    });

    expect(pushMock).toHaveBeenCalledWith("/dashboard");
    expect(useAuthStore.getState().user).toEqual({
      id: "u-1",
      username: "alice",
    });
    expect(useAuthStore.getState().isAuthenticated).toBe(true);
  });

  it("redirects to change-password when backend requires password reset", async () => {
    postMock.mockResolvedValue({
      data: {
        token: "token-2",
        force_password_change: true,
      },
    });
    getMock.mockResolvedValue({
      data: {
        id: "u-2",
        username: "bob",
      },
    });

    const { result } = renderHook(() => useAuth());
    await act(async () => {
      await result.current.login({ username: "bob", password: "secret" });
    });

    expect(pushMock).toHaveBeenCalledWith("/auth/change-password");
    expect(useAuthStore.getState().forcePasswordChange).toBe(true);
  });

  it("falls back to username-based profile when /auth/me has no data", async () => {
    postMock.mockResolvedValue({
      data: {
        token: "token-3",
        force_password_change: false,
      },
    });
    getMock.mockResolvedValue({ data: undefined });

    const { result } = renderHook(() => useAuth());
    await act(async () => {
      await result.current.login({ username: "charlie", password: "secret" });
    });

    expect(useAuthStore.getState().user).toEqual({
      id: "charlie",
      username: "charlie",
    });
    expect(pushMock).toHaveBeenCalledWith("/dashboard");
  });

  it("surfaces api errors with translated message", async () => {
    postMock.mockResolvedValue({
      error: {
        code: "INVALID_CREDENTIALS",
        message: "invalid credentials",
      },
    });

    const { result } = renderHook(() => useAuth());
    await expect(
      result.current.login({ username: "dave", password: "wrong" }),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });

    expect(messageErrorMock).toHaveBeenCalledWith("INVALID_CREDENTIALS");
    expect(tMock).toHaveBeenCalledWith("INVALID_CREDENTIALS");
  });

  it("clears auth state and redirects to login on logout", async () => {
    postMock.mockResolvedValue({});
    useAuthStore
      .getState()
      .login({ id: "u-9", username: "eve" }, false);

    const { result } = renderHook(() => useAuth());
    await act(async () => {
      await result.current.logout();
    });

    expect(useAuthStore.getState().isAuthenticated).toBe(false);
    expect(useAuthStore.getState().user).toBeNull();
    expect(postMock).toHaveBeenCalledWith("/auth/logout");
    expect(pushMock).toHaveBeenCalledWith("/login");
    expect(window.sessionStorage.getItem("shepherd-login-entry-override")).toBe("/login");
  });

  it("does nothing when login response has neither data nor error", async () => {
    postMock.mockResolvedValue({});

    const { result } = renderHook(() => useAuth());
    await act(async () => {
      await result.current.login({ username: "noop", password: "noop" });
    });

    expect(useAuthStore.getState().isAuthenticated).toBe(false);
    expect(pushMock).not.toHaveBeenCalled();
    expect(messageErrorMock).not.toHaveBeenCalled();
  });

  it("starts external login with an app-relative return_to and redirects in the current window", async () => {
    postMock.mockResolvedValue({
      data: {
        redirect_url: "https://login.example.com/start",
      },
    });

    const originalLocation = window.location;
    const assignMock = vi.fn();
    Object.defineProperty(window, "location", {
      configurable: true,
      value: {
        ...originalLocation,
        assign: assignMock,
      },
    });

    try {
      const { result } = renderHook(() => useAuth());
      await act(async () => {
        await result.current.startExternalLogin(
          "external-provider",
          "qr",
          "/dashboard",
        );
      });

      expect(postMock).toHaveBeenCalledWith(
        "/auth/providers/{provider_id}/login/start",
        expect.objectContaining({
          params: { path: { provider_id: "external-provider" } },
          body: expect.objectContaining({
            login_mode: "qr",
            return_to: "/dashboard",
          }),
        }),
      );
      expect(assignMock).toHaveBeenCalledWith("https://login.example.com/start");
      expect(useAuthStore.getState().user).toBeNull();
      expect(pushMock).not.toHaveBeenCalled();
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      });
    }
  });

  it("submits credential-based provider login and stores the returned session user", async () => {
    postMock.mockResolvedValue({
      data: {
        token: "token-ldap",
        force_password_change: false,
      },
    });
    getMock.mockResolvedValue({
      data: {
        id: "u-ldap",
        username: "alice",
      },
    });

    const { result } = renderHook(() => useAuth());
    await act(async () => {
      await result.current.submitExternalCredentialLogin(
        "ldap-provider",
        "credentials",
        { username: "alice", password: "secret" },
        "/dashboard",
      );
    });

    expect(postMock).toHaveBeenCalledWith(
      "/auth/providers/{provider_id}/login/submit",
      expect.objectContaining({
        params: { path: { provider_id: "ldap-provider" } },
        body: expect.objectContaining({
          login_mode: "credentials",
          credentials: { username: "alice", password: "secret" },
          return_to: "/dashboard",
        }),
      }),
    );
    expect(useAuthStore.getState().user).toEqual({
      id: "u-ldap",
      username: "alice",
    });
    expect(pushMock).toHaveBeenCalledWith("/dashboard");
  });
});
