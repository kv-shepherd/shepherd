import { afterEach, describe, expect, it } from "vitest";

import {
  consumeNextLoginEntry,
  getLoginEntryPath,
  getStandardLoginPath,
  normalizeLoginEntryPath,
  setNextLoginEntryOverride,
} from "./loginEntry";

describe("loginEntry", () => {
  const originalLoginEntryPath = process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH;

  afterEach(() => {
    process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH = originalLoginEntryPath;
    window.sessionStorage.clear();
  });

  it("defaults to /login", () => {
    delete process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH;
    expect(getLoginEntryPath()).toBe("/login");
  });

  it("allows an absolute app path override", () => {
    process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH = "/custom-login";
    expect(getLoginEntryPath()).toBe("/custom-login");
  });

  it("rejects invalid values", () => {
    expect(normalizeLoginEntryPath("login")).toBe("/login");
    expect(normalizeLoginEntryPath("//login")).toBe("/login");
    expect(normalizeLoginEntryPath("")).toBe("/login");
  });

  it("keeps /login as the standard explicit login path", () => {
    process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH = "/custom-login";
    expect(getStandardLoginPath()).toBe("/login");
  });

  it("consumes a one-shot login override before falling back to the default entry", () => {
    process.env.NEXT_PUBLIC_LOGIN_ENTRY_PATH = "/custom-login";
    setNextLoginEntryOverride("/login");
    expect(consumeNextLoginEntry()).toBe("/login");
    expect(consumeNextLoginEntry()).toBe("/custom-login");
  });
});
