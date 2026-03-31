/**
 * Auth hooks for login, logout, and authentication state.
 *
 * AGENTS.md §8.1: Initialize app once, not per mount.
 * Uses the generated OpenAPI types for type-safe auth calls.
 */
"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { App } from "antd";
import { useTranslation } from "react-i18next";
import { AUTH_STORAGE_KEY, useAuthStore } from "@/stores/auth";
import { api } from "@/lib/api/client";
import {
  getStandardLoginPath,
  setNextLoginEntryOverride,
} from "@/lib/auth/loginEntry";
import type { components } from "@/types/api.gen";
import type { ApiErrorResponse } from "./useApiQuery";

interface LoginPayload {
  username: string;
  password: string;
}

type UserInfo = components["schemas"]["UserInfo"];
type ExternalAuthBridgePayload = {
  type: string;
  success?: boolean;
  token?: string;
  expires_at?: string;
  user?: UserInfo;
  force_password_change?: boolean;
  code?: string;
  return_to?: string;
};

const EXTERNAL_AUTH_BRIDGE_MESSAGE_TYPE = "shepherd.external_auth.complete";

function readPersistedExternalAuthPayload(): ExternalAuthBridgePayload | null {
  if (typeof window === "undefined") {
    return null;
  }
  const raw = window.localStorage.getItem(AUTH_STORAGE_KEY);
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as {
      state?: {
        token?: unknown;
        user?: unknown;
        isAuthenticated?: unknown;
        forcePasswordChange?: unknown;
      };
    };
    const state = parsed?.state;
    if (!state || state.isAuthenticated !== true) {
      return null;
    }
    if (typeof state.token !== "string" || !state.token) {
      return null;
    }
    if (!state.user || typeof state.user !== "object") {
      return null;
    }
    return {
      type: EXTERNAL_AUTH_BRIDGE_MESSAGE_TYPE,
      success: true,
      token: state.token,
      user: state.user as UserInfo,
      force_password_change: Boolean(state.forcePasswordChange),
    };
  } catch {
    return null;
  }
}

function getExternalAuthBridgeOrigin(): string {
  if (typeof window === "undefined") {
    return "http://localhost";
  }
  try {
    return new URL(
      process.env.NEXT_PUBLIC_API_URL ?? "/api/v1",
      window.location.origin,
    ).origin;
  } catch {
    return window.location.origin;
  }
}

function normalizeExternalAuthNavigationTarget(raw?: string): string {
  if (typeof window === "undefined") {
    return "/dashboard";
  }
  const candidate = raw?.trim() || "/dashboard";
  try {
    const parsed = new URL(candidate, window.location.origin);
    if (parsed.origin !== window.location.origin) {
      return "/dashboard";
    }
    return `${parsed.pathname}${parsed.search}${parsed.hash}` || "/dashboard";
  } catch {
    return "/dashboard";
  }
}

export function useAuth() {
  const router = useRouter();
  const { message } = App.useApp();
  const { t } = useTranslation("errors");

  const {
    token,
    user,
    isAuthenticated,
    forcePasswordChange,
    login,
    logout: clearAuth,
  } = useAuthStore();

  const completeLoginWithToken = useCallback(
    async (
      token: string,
      fallbackUser: UserInfo,
      forcePasswordChange: boolean,
      successTarget: string,
    ) => {
      const { data: userInfo } = await api.GET("/auth/me", {
        headers: {
          Authorization: `Bearer ${token}`,
        },
      });

      const resolvedUser = userInfo ?? fallbackUser;
      login(token, resolvedUser, forcePasswordChange);
      if (forcePasswordChange) {
        router.push("/auth/change-password");
      } else {
        router.push(successTarget);
      }
    },
    [login, router],
  );

  const handleLogin = useCallback(
    async (payload: LoginPayload) => {
      // POST /auth/login (baseUrl already includes /api/v1)
      const { data, error } = await api.POST("/auth/login", {
        body: payload,
      });

      if (error) {
        const apiError = error as unknown as ApiErrorResponse;
        message.error(t(apiError.code ?? "INVALID_CREDENTIALS"));
        throw apiError;
      }

      if (data) {
        await completeLoginWithToken(
          data.token,
          {
            id: payload.username,
            username: payload.username,
          },
          data.force_password_change ?? false,
          "/dashboard",
        );
      }
    },
    [completeLoginWithToken, message, t],
  );

  const handleLogout = useCallback(() => {
    setNextLoginEntryOverride(getStandardLoginPath());
    clearAuth();
    router.push(getStandardLoginPath());
  }, [clearAuth, router]);

  const startExternalLogin = useCallback(
    async (providerId: string, loginMode?: string, returnTo = "/dashboard") => {
      const normalizedReturnTo = normalizeExternalAuthNavigationTarget(returnTo);
      const expectedBridgeOrigin = getExternalAuthBridgeOrigin();
      const { data, error } = await api.POST(
        "/auth/providers/{provider_id}/login/start",
        {
          params: { path: { provider_id: providerId } },
          body: {
            login_mode: loginMode,
            return_to: normalizedReturnTo,
          },
        },
      );

      if (error || !data?.redirect_url) {
        const apiError = error as unknown as ApiErrorResponse | undefined;
        const normalized = apiError ?? { code: "EXTERNAL_AUTH_START_FAILED" };
        message.error(t(normalized.code ?? "INTERNAL_ERROR"));
        throw normalized;
      }

      const popup = window.open(
        data.redirect_url,
        `shepherd-external-auth-${providerId}`,
        "popup=yes,width=520,height=720",
      );
      if (!popup) {
        const popupError: ApiErrorResponse = { code: "POPUP_BLOCKED" };
        message.error(t(popupError.code));
        throw popupError;
      }

      await new Promise<void>((resolve, reject) => {
        let settled = false;
        const cleanup = () => {
          window.removeEventListener("message", onMessage);
          window.removeEventListener("storage", onStorage);
          window.clearInterval(closedPoll);
        };

        const fail = (err: ApiErrorResponse) => {
          if (settled) return;
          settled = true;
          cleanup();
          try {
            popup.close();
          } catch {
            // ignore
          }
          reject(err);
        };

        const succeed = (payload: ExternalAuthBridgePayload) => {
          if (settled) return;
          settled = true;
          cleanup();
          login(
            payload.token as string,
            payload.user as UserInfo,
            payload.force_password_change ?? false,
          );
          if (payload.force_password_change) {
            router.push("/auth/change-password");
          } else {
            router.push(
              normalizeExternalAuthNavigationTarget(
                payload.return_to || returnTo,
              ),
            );
          }
          try {
            popup.close();
          } catch {
            // ignore
          }
          resolve();
        };

        const recoverPersistedLogin = () => {
          const persisted = readPersistedExternalAuthPayload();
          if (!persisted) {
            return false;
          }
          succeed({
            ...persisted,
            return_to: persisted.return_to || returnTo,
          });
          return true;
        };

        const onMessage = (event: MessageEvent<ExternalAuthBridgePayload>) => {
          if (event.origin !== expectedBridgeOrigin) return;
          const payload = event.data;
          if (!payload || payload.type !== EXTERNAL_AUTH_BRIDGE_MESSAGE_TYPE)
            return;
          if (payload.success === false || payload.code) {
            fail({ code: payload.code || "EXTERNAL_AUTH_CALLBACK_FAILED" });
            return;
          }
          if (!payload.token || !payload.user) {
            fail({ code: "EXTERNAL_AUTH_CALLBACK_FAILED" });
            return;
          }
          succeed(payload);
        };

        const onStorage = (event: StorageEvent) => {
          if (event.key !== AUTH_STORAGE_KEY) {
            return;
          }
          recoverPersistedLogin();
        };

        const closedPoll = window.setInterval(() => {
          if (popup.closed) {
            if (recoverPersistedLogin()) {
              return;
            }
            fail({ code: "EXTERNAL_AUTH_CANCELLED" });
          }
        }, 500);

        window.addEventListener("message", onMessage);
        window.addEventListener("storage", onStorage);
      });
    },
    [login, message, router, t],
  );

  const submitExternalCredentialLogin = useCallback(
    async (
      providerId: string,
      loginMode: string | undefined,
      credentials: Record<string, unknown>,
      returnTo = "/dashboard",
    ) => {
      const normalizedReturnTo = normalizeExternalAuthNavigationTarget(returnTo);
      const { data, error } = await api.POST(
        "/auth/providers/{provider_id}/login/submit",
        {
          params: { path: { provider_id: providerId } },
          body: {
            login_mode: loginMode,
            credentials,
            return_to: normalizedReturnTo,
          },
        },
      );

      if (error || !data?.token) {
        const apiError = error as unknown as ApiErrorResponse | undefined;
        const normalized = apiError ?? { code: "INVALID_CREDENTIALS" };
        message.error(t(normalized.code ?? "INTERNAL_ERROR"));
        throw normalized;
      }

      const fallbackUsername =
        typeof credentials.username === "string" ? credentials.username : providerId;
      await completeLoginWithToken(
        data.token,
        {
          id: fallbackUsername,
          username: fallbackUsername,
        },
        data.force_password_change ?? false,
        normalizedReturnTo,
      );
    },
    [completeLoginWithToken, message, t],
  );

  return {
    token,
    user,
    isAuthenticated,
    forcePasswordChange,
    login: handleLogin,
    startExternalLogin,
    submitExternalCredentialLogin,
    logout: handleLogout,
  };
}
