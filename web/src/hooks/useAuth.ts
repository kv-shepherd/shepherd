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
import { useAuthStore } from "@/stores/auth";
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
      window.location.assign(data.redirect_url);
    },
    [message, t],
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
