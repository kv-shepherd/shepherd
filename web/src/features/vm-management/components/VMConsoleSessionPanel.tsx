"use client";

import { useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Space, Spin, Tag, Typography } from "antd";
import { RedoOutlined } from "@ant-design/icons";
import { useTranslation } from "react-i18next";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";

import type {
  ResolvedConsoleTarget,
  VMConsoleType,
} from "@/features/vm-management/console";
import { api } from "@/lib/api/client";
import { translateApiError } from "@/lib/api/errorMessage";

const { Paragraph, Text } = Typography;

type RFBModule = {
  default: new (
    target: HTMLElement,
    url: string,
    options?: Record<string, unknown>,
  ) => {
    scaleViewport: boolean;
    resizeSession: boolean;
    viewOnly: boolean;
    disconnect: () => void;
    addEventListener: (
      type: string,
      listener: EventListenerOrEventListenerObject,
    ) => void;
  };
};

type RFBConnection = InstanceType<RFBModule["default"]>;

interface VMConsoleSessionPanelProps {
  vmId: string;
  target: ResolvedConsoleTarget | null;
  onReconnectConsole?: (consoleType: VMConsoleType) => Promise<boolean>;
}

const connectionClosedMessage = "\r\n\x1b[31mConnection closed\x1b[0m";

const toWebSocketURL = (path: string): string => {
  const normalized = path.startsWith("/") ? path : `/${path}`;
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}${normalized}`;
};

function SerialConsoleSession({
  vmId,
  path,
  onReconnectConsole,
}: {
  vmId: string;
  path: string;
  onReconnectConsole?: (consoleType: VMConsoleType) => Promise<boolean>;
}) {
  const { t } = useTranslation(["vm", "common"]);
  const [errorMessage, setErrorMessage] = useState("");
  const [loading, setLoading] = useState(true);
  const [connected, setConnected] = useState(false);
  const [reconnectTick, setReconnectTick] = useState(0);
  const [reconnecting, setReconnecting] = useState(false);
  const [sessionClosed, setSessionClosed] = useState(false);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const connectedRef = useRef(false);
  const receivedOutputRef = useRef(false);

  useEffect(() => {
    let cancelled = false;
    let socket: WebSocket | undefined;
    let terminal: Terminal | undefined;
    let fitAddon: FitAddon | undefined;
    let resizeObserver: ResizeObserver | undefined;
    let disposeInput: { dispose: () => void } | undefined;
    connectedRef.current = false;
    receivedOutputRef.current = false;

    const writeIncomingData = async (payload: unknown) => {
      if (!terminal || cancelled) {
        return;
      }
      if (typeof payload === "string") {
        receivedOutputRef.current = true;
        terminal.write(payload);
        return;
      }
      if (payload instanceof ArrayBuffer) {
        receivedOutputRef.current = true;
        terminal.write(new Uint8Array(payload));
        return;
      }
      if (payload instanceof Blob) {
        const bytes = new Uint8Array(await payload.arrayBuffer());
        if (!cancelled) {
          receivedOutputRef.current = true;
          terminal.write(bytes);
        }
      }
    };

    const connect = async () => {
      try {
        const { data, error } = await api.GET("/vms/{vm_id}/serial", {
          params: { path: { vm_id: vmId } },
        });
        if (cancelled) {
          return;
        }
        if (error) {
          setErrorMessage(translateApiError(t, error));
          setLoading(false);
          return;
        }
        if (!containerRef.current) {
          setErrorMessage(t("console.unavailable"));
          setLoading(false);
          return;
        }

        terminal = new Terminal({
          convertEol: true,
          cursorBlink: true,
          fontSize: 13,
          theme: {
            background: "#0f172a",
            foreground: "#e2e8f0",
            cursor: "#93c5fd",
          },
        });
        fitAddon = new FitAddon();
        terminal.loadAddon(fitAddon);
        terminal.open(containerRef.current);
        fitAddon.fit();

        const websocketPath = (data?.websocket_path ?? path).trim();
        socket = new WebSocket(toWebSocketURL(websocketPath));
        socket.binaryType = "arraybuffer";

        disposeInput = terminal.onData((input: string) => {
          if (socket?.readyState === WebSocket.OPEN) {
            socket.send(input);
          }
        });

        resizeObserver = new ResizeObserver(() => {
          fitAddon?.fit();
        });
        resizeObserver.observe(containerRef.current);

        socket.addEventListener("open", () => {
          if (cancelled) {
            return;
          }
          connectedRef.current = true;
          setConnected(true);
          setLoading(false);
          setSessionClosed(false);
          setReconnecting(false);
          fitAddon?.fit();
          terminal?.focus();
          window.setTimeout(() => {
            if (
              !cancelled &&
              !receivedOutputRef.current &&
              socket?.readyState === WebSocket.OPEN
            ) {
              socket.send("\r");
            }
          }, 120);
        });

        socket.addEventListener("message", (event) => {
          void writeIncomingData(event.data);
        });

        socket.addEventListener("close", () => {
          if (cancelled) {
            return;
          }
          connectedRef.current = false;
          setConnected(false);
          setLoading(false);
          setSessionClosed(true);
          setReconnecting(false);
          terminal?.write(connectionClosedMessage);
        });

        socket.addEventListener("error", () => {
          if (cancelled) {
            return;
          }
          if (!connectedRef.current) {
            setErrorMessage(t("console.unavailable"));
            setLoading(false);
            setSessionClosed(true);
            setReconnecting(false);
          }
        });
      } catch (error) {
        if (!cancelled) {
          setErrorMessage(
            error instanceof Error ? error.message : t("console.unavailable"),
          );
          setLoading(false);
          setReconnecting(false);
        }
      }
    };

    void connect();

    return () => {
      cancelled = true;
      resizeObserver?.disconnect();
      disposeInput?.dispose();
      socket?.close();
      terminal?.dispose();
    };
  }, [path, reconnectTick, t, vmId]);

  const reconnectSerialConsole = async () => {
    setErrorMessage("");
    setConnected(false);
    setLoading(true);
    setReconnecting(true);
    setSessionClosed(false);
    if (onReconnectConsole) {
      const refreshed = await onReconnectConsole("SERIAL");
      if (!refreshed) {
        setLoading(false);
        setReconnecting(false);
        setSessionClosed(true);
        return;
      }
    }
    setReconnectTick((value) => value + 1);
  };

  return (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Space style={{ width: "100%", justifyContent: "space-between" }} wrap>
        <Space size="small" wrap>
          <Text strong>{t("console.option_serial_title")}</Text>
          <Tag color="green">{t("console.serial_available")}</Tag>
        </Space>
        {sessionClosed || errorMessage ? (
          <Button
            icon={<RedoOutlined />}
            loading={reconnecting}
            onClick={() => void reconnectSerialConsole()}
          >
            {t("action.reconnect_serial")}
          </Button>
        ) : null}
      </Space>
      {errorMessage ? (
        <Alert
          type="error"
          showIcon={true}
          message={t("common:message.error")}
          description={errorMessage}
        />
      ) : null}
      {!errorMessage ? (
        <Card
          styles={{
            body: {
              minHeight: "70vh",
              padding: 12,
            },
          }}
        >
          {loading ? (
            <div
              style={{
                minHeight: "68vh",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Space direction="vertical" align="center" size={12}>
                <Spin size="large" />
                <Text type="secondary">{t("console.connecting")}</Text>
              </Space>
            </div>
          ) : null}
          <div
            ref={containerRef}
            data-testid="serial-console-terminal"
            style={{
              display: loading ? "none" : "block",
              width: "100%",
              minHeight: "68vh",
              background: "#0f172a",
            }}
          />
        </Card>
      ) : null}
      {!loading && connected ? null : !errorMessage ? (
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          {t("console.cookie_hint")}
        </Paragraph>
      ) : null}
      {sessionClosed && !errorMessage ? (
        <Alert
          type="info"
          showIcon={true}
          message={t("action.reconnect_serial")}
          description={t("console.reconnect_hint")}
        />
      ) : null}
    </Space>
  );
}

function VNCConsoleSession({
  vmId,
  path,
}: {
  vmId: string;
  path: string;
}) {
  const { t } = useTranslation(["vm", "common"]);
  const [errorMessage, setErrorMessage] = useState("");
  const [loading, setLoading] = useState(true);
  const [connected, setConnected] = useState(false);
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    let rfb: RFBConnection | undefined;

    const connect = async () => {
      try {
        const { data, error } = await api.GET("/vms/{vm_id}/vnc", {
          params: { path: { vm_id: vmId } },
        });
        if (cancelled) {
          return;
        }
        if (error) {
          setErrorMessage(translateApiError(t, error));
          setLoading(false);
          return;
        }

        const websocketPath = (data?.websocket_path ?? path).trim();
        if (!containerRef.current) {
          setErrorMessage(t("console.unavailable"));
          setLoading(false);
          return;
        }

        const { default: RFB } = (await import(
          "@novnc/novnc/lib/rfb"
        )) as RFBModule;
        if (cancelled || !containerRef.current) {
          return;
        }

        rfb = new RFB(containerRef.current, toWebSocketURL(websocketPath));
        rfb.scaleViewport = true;
        rfb.resizeSession = false;
        rfb.viewOnly = false;
        rfb.addEventListener("connect", () => {
          if (!cancelled) {
            setConnected(true);
            setLoading(false);
          }
        });
        rfb.addEventListener("disconnect", () => {
          if (!cancelled) {
            setConnected(false);
            setLoading(false);
          }
        });
      } catch (error) {
        if (!cancelled) {
          setErrorMessage(
            error instanceof Error ? error.message : t("console.unavailable"),
          );
          setLoading(false);
        }
      }
    };

    void connect();

    return () => {
      cancelled = true;
      rfb?.disconnect();
    };
  }, [path, t, vmId]);

  return (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Space size="small" wrap>
        <Text strong>{t("console.option_vnc_title")}</Text>
        <Tag color="blue">{t("console.vnc_available")}</Tag>
      </Space>
      {errorMessage ? (
        <Alert
          type="error"
          showIcon={true}
          message={t("common:message.error")}
          description={errorMessage}
        />
      ) : null}
      {!errorMessage ? (
        <Card
          styles={{
            body: {
              minHeight: "70vh",
              padding: 12,
            },
          }}
        >
          {loading ? (
            <div
              style={{
                minHeight: "68vh",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Space direction="vertical" align="center" size={12}>
                <Spin size="large" />
                <Text type="secondary">{t("console.connecting")}</Text>
              </Space>
            </div>
          ) : null}
          <div
            ref={containerRef}
            data-testid="vnc-canvas"
            style={{
              display: loading ? "none" : "block",
              width: "100%",
              minHeight: "68vh",
              background: "#111827",
            }}
          />
        </Card>
      ) : null}
      {!loading && connected ? null : !errorMessage ? (
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          {t("console.cookie_hint")}
        </Paragraph>
      ) : null}
    </Space>
  );
}

export function VMConsoleSessionPanel({
  vmId,
  target,
  onReconnectConsole,
}: VMConsoleSessionPanelProps) {
  const { t } = useTranslation(["vm"]);

  if (!target) {
    return (
      <Space direction="vertical" size={8} style={{ width: "100%" }}>
        <Text strong>{t("field.console_access")}</Text>
        <Text type="secondary">{t("console.embedded_idle")}</Text>
      </Space>
    );
  }

  if (target.consoleType === "SERIAL") {
    return (
      <SerialConsoleSession
        vmId={vmId}
        path={target.consolePath}
        onReconnectConsole={onReconnectConsole}
      />
    );
  }

  return <VNCConsoleSession vmId={vmId} path={target.consolePath} />;
}
