import type { VM } from "@/features/vm-management/types";
import type { components } from "@/types/api.gen";

export type VMConsoleType = components["schemas"]["VMConsoleType"];
export interface VMConsoleCapabilities {
  serial_available?: boolean;
  vnc_available?: boolean;
  preferred_console_type?: VMConsoleType | null;
}

const normalizeOSValue = (value?: string | null): string =>
  value?.trim().toLowerCase() ?? "";

const preferredConsoleTypeStorageKey = "vm.console.preferred_type";

interface ConsoleLaunchPayload {
  console_type?: VMConsoleType | null;
  console_url?: string | null;
  vnc_url?: string | null;
}

export interface ResolvedConsoleTarget {
  consoleType: VMConsoleType;
  consolePath: string;
}

export const isConsoleTypeAvailable = (
  capabilities: VMConsoleCapabilities | null | undefined,
  consoleType: VMConsoleType,
): boolean => {
  if (!capabilities) {
    return true;
  }
  if (consoleType === "SERIAL") {
    return capabilities.serial_available !== false;
  }
  return capabilities.vnc_available !== false;
};

export const hasAnyConsoleCapability = (
  vm?: Partial<VM> | null,
): boolean => {
  const capabilities = vm?.console_capabilities;
  if (capabilities) {
    return Boolean(capabilities.serial_available || capabilities.vnc_available);
  }

  const osFamily = normalizeOSValue(vm?.os_family);
  const osName = normalizeOSValue(vm?.os_name);
  if (osFamily === "windows" || osName.includes("windows")) {
    return false;
  }
  return true;
};

export const readStoredPreferredConsoleType = (): VMConsoleType | null => {
  if (typeof window === "undefined") {
    return null;
  }
  try {
    const value = window.localStorage.getItem(preferredConsoleTypeStorageKey);
    return value === "SERIAL" || value === "VNC" ? value : null;
  } catch {
    return null;
  }
};

export const saveStoredPreferredConsoleType = (consoleType: VMConsoleType) => {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(preferredConsoleTypeStorageKey, consoleType);
  } catch {
    // Ignore storage failures and keep the connection flow working.
  }
};

export const resolveDefaultConsoleType = (
  capabilities: VMConsoleCapabilities | null | undefined,
  storedPreference?: VMConsoleType | null,
): VMConsoleType => {
  if (storedPreference && isConsoleTypeAvailable(capabilities, storedPreference)) {
    return storedPreference;
  }
  const preferredConsoleType = capabilities?.preferred_console_type;
  if (
    preferredConsoleType &&
    isConsoleTypeAvailable(capabilities, preferredConsoleType)
  ) {
    return preferredConsoleType;
  }
  if (isConsoleTypeAvailable(capabilities, "SERIAL")) {
    return "SERIAL";
  }
  return "VNC";
};

const absoluteURLPattern = /^[a-z][a-z0-9+.-]*:/i;

const consolePathPatterns: Record<VMConsoleType, RegExp> = {
  SERIAL: /^\/api\/v1\/vms\/[^/?#]+\/serial$/,
  VNC: /^\/api\/v1\/vms\/[^/?#]+\/vnc$/,
};

const normalizeConsolePath = (
  path: string,
  consoleType: VMConsoleType,
): string | null => {
  const trimmed = path.trim();
  if (
    trimmed === "" ||
    trimmed.startsWith("//") ||
    absoluteURLPattern.test(trimmed)
  ) {
    return null;
  }
  const normalized = trimmed.startsWith("/") ? trimmed : `/${trimmed}`;
  return consolePathPatterns[consoleType].test(normalized) ? normalized : null;
};

export const resolveApprovedConsoleTarget = (
  payload?: ConsoleLaunchPayload | null,
): ResolvedConsoleTarget | null => {
  const consolePath =
    typeof payload?.console_url === "string" ? payload.console_url.trim() : "";
  const consoleType = payload?.console_type;
  if (consoleType === "SERIAL" || consoleType === "VNC") {
    const normalizedConsolePath = normalizeConsolePath(consolePath, consoleType);
    if (normalizedConsolePath) {
      return {
        consoleType,
        consolePath: normalizedConsolePath,
      };
    }
  }

  const legacyVNCPath =
    typeof payload?.vnc_url === "string" ? payload.vnc_url.trim() : "";
  const normalizedLegacyVNCPath = normalizeConsolePath(legacyVNCPath, "VNC");
  if (normalizedLegacyVNCPath) {
    return {
      consoleType: "VNC",
      consolePath: normalizedLegacyVNCPath,
    };
  }

  return null;
};
