const DEFAULT_LOGIN_ENTRY_PATH = "/login";
const LOGIN_ENTRY_OVERRIDE_KEY = "shepherd-login-entry-override";

export function normalizeLoginEntryPath(raw?: string | null): string {
  const candidate = (raw ?? "").trim();
  if (!candidate) {
    return DEFAULT_LOGIN_ENTRY_PATH;
  }
  if (!candidate.startsWith("/")) {
    return DEFAULT_LOGIN_ENTRY_PATH;
  }
  if (candidate.startsWith("//")) {
    return DEFAULT_LOGIN_ENTRY_PATH;
  }
  return candidate;
}

export function getLoginEntryPath(): string {
  return normalizeLoginEntryPath(process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH);
}

export function getStandardLoginPath(): string {
  return DEFAULT_LOGIN_ENTRY_PATH;
}

export function setNextLoginEntryOverride(path?: string | null): void {
  if (typeof window === "undefined") {
    return;
  }
  const normalized = normalizeLoginEntryPath(path);
  window.sessionStorage.setItem(LOGIN_ENTRY_OVERRIDE_KEY, normalized);
}

export function consumeNextLoginEntry(): string {
  if (typeof window === "undefined") {
    return getLoginEntryPath();
  }
  const overridden = window.sessionStorage.getItem(LOGIN_ENTRY_OVERRIDE_KEY);
  if (overridden) {
    window.sessionStorage.removeItem(LOGIN_ENTRY_OVERRIDE_KEY);
    return normalizeLoginEntryPath(overridden);
  }
  return getLoginEntryPath();
}
