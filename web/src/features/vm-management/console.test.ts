import { describe, expect, it, beforeEach } from "vitest";

import {
  hasAnyConsoleCapability,
  readStoredPreferredConsoleType,
  resolveApprovedConsoleTarget,
  resolveDefaultConsoleType,
  saveStoredPreferredConsoleType,
} from "./console";

describe("vm console helpers", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("prefers the stored console type when it is still available", () => {
    saveStoredPreferredConsoleType("VNC");

    expect(
      resolveDefaultConsoleType(
        {
          serial_available: true,
          vnc_available: true,
          preferred_console_type: "SERIAL",
        },
        readStoredPreferredConsoleType(),
      ),
    ).toBe("VNC");
  });

  it("falls back to the live preferred type when the stored one is unavailable", () => {
    saveStoredPreferredConsoleType("VNC");

    expect(
      resolveDefaultConsoleType(
        {
          serial_available: true,
          vnc_available: false,
          preferred_console_type: "SERIAL",
        },
        readStoredPreferredConsoleType(),
      ),
    ).toBe("SERIAL");
  });

  it("resolves approved serial console responses into an embedded target", () => {
    expect(
      resolveApprovedConsoleTarget({
        console_type: "SERIAL",
        console_url: "/api/v1/vms/vm-1/serial",
      }),
    ).toEqual({
      consoleType: "SERIAL",
      consolePath: "/api/v1/vms/vm-1/serial",
    });
  });

  it("normalizes relative same-origin console paths", () => {
    expect(
      resolveApprovedConsoleTarget({
        console_type: "VNC",
        console_url: "api/v1/vms/vm-1/vnc",
      }),
    ).toEqual({
      consoleType: "VNC",
      consolePath: "/api/v1/vms/vm-1/vnc",
    });
  });

  it.each([
    {
      name: "non-console API path",
      payload: {
        console_type: "SERIAL" as const,
        console_url: "/api/v1/admin/observability/traces",
      },
    },
    {
      name: "console path with query",
      payload: {
        console_type: "VNC" as const,
        console_url: "/api/v1/vms/vm-1/vnc?token=leaky",
      },
    },
    {
      name: "console path with fragment",
      payload: {
        console_type: "SERIAL" as const,
        console_url: "/api/v1/vms/vm-1/serial#top",
      },
    },
  ])("rejects $name", ({ payload }) => {
    expect(resolveApprovedConsoleTarget(payload)).toBeNull();
  });

  it.each([
    {
      name: "serial type with vnc endpoint",
      payload: {
        console_type: "SERIAL" as const,
        console_url: "/api/v1/vms/vm-1/vnc",
      },
    },
    {
      name: "vnc type with serial endpoint",
      payload: {
        console_type: "VNC" as const,
        console_url: "/api/v1/vms/vm-1/serial",
      },
    },
    {
      name: "legacy vnc_url with serial endpoint",
      payload: {
        vnc_url: "/api/v1/vms/vm-1/serial",
      },
    },
  ])("rejects $name", ({ payload }) => {
    expect(resolveApprovedConsoleTarget(payload)).toBeNull();
  });

  it("rejects absolute or protocol-relative console paths", () => {
    expect(
      resolveApprovedConsoleTarget({
        console_type: "SERIAL",
        console_url: "https://console.example.test/vms/vm-1/serial",
      }),
    ).toBeNull();
    expect(
      resolveApprovedConsoleTarget({
        console_type: "VNC",
        console_url: "//console.example.test/vms/vm-1/vnc",
      }),
    ).toBeNull();
    expect(
      resolveApprovedConsoleTarget({
        vnc_url: "wss://console.example.test/vms/vm-1/vnc",
      }),
    ).toBeNull();
  });

  it("disables console entry conservatively for windows VMs without live capability hints", () => {
    expect(
      hasAnyConsoleCapability({
        os_family: "windows",
        os_name: "Windows Server 2025",
      }),
    ).toBe(false);
  });

  it("keeps console entry available by default for non-windows VMs without live capability hints", () => {
    expect(
      hasAnyConsoleCapability({
        os_family: "linux",
        os_name: "Ubuntu 24.04 LTS",
      }),
    ).toBe(true);
  });
});
