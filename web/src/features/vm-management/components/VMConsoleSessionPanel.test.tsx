import { StrictMode } from "react";
import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { VMConsoleSessionPanel } from "./VMConsoleSessionPanel";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("@xterm/addon-fit", () => ({
  FitAddon: class {
    fit() {}
  },
}));

vi.mock("@xterm/xterm", () => ({
  Terminal: class {
    loadAddon() {}
    open() {}
    focus() {}
    write() {}
    onData() {
      return { dispose() {} };
    }
    dispose() {}
  },
}));

class MockResizeObserver {
  observe() {}
  disconnect() {}
}

class MockWebSocket {
  static instances: MockWebSocket[] = [];
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  readonly url: string;
  readyState = MockWebSocket.CONNECTING;
  binaryType = "blob";
  private listeners = new Map<string, Set<EventListenerOrEventListenerObject>>();

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    const set = this.listeners.get(type) ?? new Set<EventListenerOrEventListenerObject>();
    set.add(listener);
    this.listeners.set(type, set);
  }

  removeEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    this.listeners.get(type)?.delete(listener);
  }

  send() {}

  close() {
    this.readyState = MockWebSocket.CLOSED;
  }
}

describe("VMConsoleSessionPanel", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    MockWebSocket.instances = [];
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("WebSocket", MockWebSocket);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("does not establish duplicate serial sessions under StrictMode", async () => {
    render(
      <StrictMode>
        <VMConsoleSessionPanel
          vmId="vm-1"
          target={{
            consoleType: "SERIAL",
            consolePath: "/api/v1/vms/vm-1/serial",
          }}
        />
      </StrictMode>,
    );

    await vi.runAllTimersAsync();
    await Promise.resolve();
    await Promise.resolve();

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0]?.url).toContain("/api/v1/vms/vm-1/serial");
  });

  it("does not reopen the serial session when the parent rerenders with the same target", async () => {
    const { rerender } = render(
      <VMConsoleSessionPanel
        vmId="vm-1"
        target={{
          consoleType: "SERIAL",
          consolePath: "/api/v1/vms/vm-1/serial",
        }}
      />,
    );

    await vi.runAllTimersAsync();
    await Promise.resolve();
    expect(MockWebSocket.instances).toHaveLength(1);

    rerender(
      <VMConsoleSessionPanel
        vmId="vm-1"
        target={{
          consoleType: "SERIAL",
          consolePath: "/api/v1/vms/vm-1/serial",
        }}
      />,
    );

    await vi.runAllTimersAsync();
    await Promise.resolve();
    expect(MockWebSocket.instances).toHaveLength(1);
  });
});
