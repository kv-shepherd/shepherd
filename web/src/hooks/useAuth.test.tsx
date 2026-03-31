import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AUTH_STORAGE_KEY, useAuthStore } from "@/stores/auth";

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
    token: null,
    user: null,
    isAuthenticated: false,
    forcePasswordChange: false,
  });
}

describe("useAuth", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    resetAuthStore();
  });

  it("stores token and redirects to dashboard on successful login", async () => {
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
    expect(useAuthStore.getState().token).toBe("token-1");
    expect(useAuthStore.getState().user).toEqual({
      id: "u-1",
      username: "alice",
    });
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

  it("clears auth state and redirects to login on logout", () => {
    useAuthStore
      .getState()
      .login("token-logout", { id: "u-9", username: "eve" }, false);

    const { result } = renderHook(() => useAuth());
    act(() => {
      result.current.logout();
    });

    expect(useAuthStore.getState().isAuthenticated).toBe(false);
    expect(useAuthStore.getState().token).toBeNull();
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

  it("starts external login with an app-relative return_to and accepts bridge messages from api origin", async () => {
    const originalApiUrl = process.env.NEXT_PUBLIC_API_URL;
    process.env.NEXT_PUBLIC_API_URL = "https://api.example.com/api/v1";

    postMock.mockResolvedValue({
      data: {
        redirect_url: "https://login.example.com/start",
      },
    });

    const popup = {
      closed: false,
      close: vi.fn(),
    };
    const openSpy = vi
      .spyOn(window, "open")
      .mockImplementation(() => popup as unknown as Window);

    const { result } = renderHook(() => useAuth());
    await act(async () => {
      const promise = result.current.startExternalLogin(
        "external-provider",
        "qr",
        "/dashboard",
      );
      await Promise.resolve();
      window.dispatchEvent(
        new MessageEvent("message", {
          origin: "https://api.example.com",
          data: {
            type: "shepherd.external_auth.complete",
            token: "token-ext",
            user: {
              id: "u-ext",
              username: "external-user",
            },
            return_to: `${window.location.origin}/dashboard`,
          },
        }),
      );
      await promise;
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
    expect(useAuthStore.getState().token).toBe("token-ext");
    expect(pushMock).toHaveBeenCalledWith("/dashboard");

    openSpy.mockRestore();
    process.env.NEXT_PUBLIC_API_URL = originalApiUrl;
  });

  it("recovers external login from persisted storage when the popup closes before postMessage is observed", async () => {
    vi.useFakeTimers();
    postMock.mockResolvedValue({
      data: {
        redirect_url: "https://login.example.com/start",
      },
    });

    const popup = {
      closed: false,
      close: vi.fn(),
    };
    const openSpy = vi
      .spyOn(window, "open")
      .mockImplementation(() => popup as unknown as Window);

    const { result } = renderHook(() => useAuth());
    let promise: Promise<void>;
    await act(async () => {
      promise = result.current.startExternalLogin(
        "external-provider",
        "redirect",
        "/dashboard",
      );
      await Promise.resolve();
      const payload = {
        state: {
          token: "token-storage",
          user: {
            id: "u-storage",
            username: "storage-user",
          },
          isAuthenticated: true,
        },
        version: 0,
      };
      window.localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(payload));
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: AUTH_STORAGE_KEY,
          newValue: JSON.stringify(payload),
        }),
      );
      popup.closed = true;
      await vi.runAllTimersAsync();
      await promise!;
    });

    expect(useAuthStore.getState().token).toBe("token-storage");
    expect(pushMock).toHaveBeenCalledWith("/dashboard");

    openSpy.mockRestore();
    vi.useRealTimers();
  });

  it("submits credential-based provider login and stores the returned token", async () => {
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
    expect(useAuthStore.getState().token).toBe("token-ldap");
    expect(pushMock).toHaveBeenCalledWith("/dashboard");
  });
});
