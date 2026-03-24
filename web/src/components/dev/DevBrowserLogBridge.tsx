"use client";

import { useEffect } from "react";

type BrowserLogLevel = "warn" | "error";

interface BrowserLogPayload {
  level: BrowserLogLevel;
  page: string;
  userAgent: string;
  timestamp: string;
  args: string[];
}

const MAX_LOG_ARG_LENGTH = 4000;

function truncateString(input: string): string {
  if (input.length <= MAX_LOG_ARG_LENGTH) {
    return input;
  }
  return `${input.slice(0, MAX_LOG_ARG_LENGTH)}…`;
}

function serializeConsoleArg(value: unknown): string {
  if (typeof value === "string") {
    return truncateString(value);
  }
  if (
    typeof value === "number" ||
    typeof value === "boolean" ||
    value === null ||
    value === undefined
  ) {
    return String(value);
  }
  if (value instanceof Error) {
    return truncateString(value.stack || value.message || value.name);
  }
  if (typeof value === "function") {
    return `[function ${value.name || "anonymous"}]`;
  }

  try {
    const seen = new WeakSet<object>();
    return truncateString(
      JSON.stringify(value, (_key, currentValue: unknown) => {
        if (typeof currentValue === "bigint") {
          return currentValue.toString();
        }
        if (typeof currentValue === "symbol") {
          return currentValue.toString();
        }
        if (typeof currentValue === "function") {
          return `[function ${currentValue.name || "anonymous"}]`;
        }
        if (
          currentValue &&
          typeof currentValue === "object"
        ) {
          if (seen.has(currentValue)) {
            return "[Circular]";
          }
          seen.add(currentValue);
        }
        return currentValue;
      }),
    );
  } catch {
    return truncateString(String(value));
  }
}

function sendBrowserLog(payload: BrowserLogPayload): void {
  const body = JSON.stringify(payload);

  try {
    if (typeof navigator !== "undefined" && typeof navigator.sendBeacon === "function") {
      const blob = new Blob([body], { type: "application/json" });
      if (navigator.sendBeacon("/api/dev/browser-log", blob)) {
        return;
      }
    }
  } catch {
    // Fall through to fetch keepalive without surfacing extra client noise.
  }

  void fetch("/api/dev/browser-log", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
    keepalive: true,
  }).catch(() => {
    // Intentionally swallow bridge transport errors. Browser console remains intact.
  });
}

export function DevBrowserLogBridge() {
  useEffect(() => {
    const originalWarn = console.warn;
    const originalError = console.error;

    const mirror =
      (level: BrowserLogLevel, original: typeof console.warn) =>
      (...args: unknown[]) => {
        original(...args);
        sendBrowserLog({
          level,
          page: window.location.href,
          userAgent: navigator.userAgent,
          timestamp: new Date().toISOString(),
          args: args.map(serializeConsoleArg),
        });
      };

    console.warn = mirror("warn", originalWarn);
    console.error = mirror("error", originalError);

    return () => {
      console.warn = originalWarn;
      console.error = originalError;
    };
  }, []);

  return null;
}
