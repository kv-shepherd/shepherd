"use client";

import { Modal, Radio, Space, Tag, Typography } from "antd";
import type { RadioChangeEvent } from "antd";
import { useTranslation } from "react-i18next";

import {
  buildVMRemoteAccessCommand,
  describeVMRemoteAccess,
  resolveVMRemoteAccessMode,
} from "@/features/vm-management/osInfo";
import type { VM } from "@/features/vm-management/types";
import type {
  VMConsoleCapabilities,
  VMConsoleType,
} from "@/features/vm-management/console";
import { isConsoleTypeAvailable } from "@/features/vm-management/console";

const { Paragraph, Text } = Typography;

interface ConsoleModeModalProps {
  open: boolean;
  loading?: boolean;
  vmName?: string | null;
  vm?: Partial<VM> | null;
  capabilities?: VMConsoleCapabilities | null;
  value: VMConsoleType;
  onCancel: () => void;
  onChange: (consoleType: VMConsoleType) => void;
  onConfirm: () => void;
}

export function ConsoleModeModal({
  open,
  loading = false,
  vmName,
  vm,
  capabilities,
  value,
  onCancel,
  onChange,
  onConfirm,
}: ConsoleModeModalProps) {
  const { t } = useTranslation(["vm", "common"]);
  const serialAvailable = isConsoleTypeAvailable(capabilities, "SERIAL");
  const vncAvailable = isConsoleTypeAvailable(capabilities, "VNC");
  const remoteAccessMode = resolveVMRemoteAccessMode(vm);
  const remoteAccessCommand = buildVMRemoteAccessCommand(vm);
  const remoteAccessDescription = describeVMRemoteAccess(t, vm);

  const handleChange = (event: RadioChangeEvent) => {
    const nextValue = event.target.value;
    if (nextValue === "SERIAL" || nextValue === "VNC") {
      onChange(nextValue);
    }
  };

  return (
    <Modal
      title={t("console.chooser_title")}
      open={open}
      onCancel={onCancel}
      onOk={onConfirm}
      confirmLoading={loading}
      okText={t("common:button.confirm")}
      cancelText={t("common:button.cancel")}
      maskClosable={false}
      keyboard={false}
      destroyOnHidden={true}
    >
      <Space direction="vertical" size={16} style={{ width: "100%" }}>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          {t("console.chooser_subtitle", {
            name: vmName || t("field.name"),
          })}
        </Paragraph>
        <Radio.Group
          value={value}
          onChange={handleChange}
          style={{ width: "100%" }}
        >
          <Space direction="vertical" size={12} style={{ width: "100%" }}>
            <div className="vm-console-option">
              <Radio value="SERIAL" disabled={!serialAvailable}>
                <Space direction="vertical" size={2}>
                  <Text strong>{t("console.option_serial_title")}</Text>
                  <Text type="secondary">
                    {t("console.option_serial_description")}
                  </Text>
                </Space>
              </Radio>
              <Tag color={serialAvailable ? "green" : "default"}>
                {serialAvailable
                  ? t("console.serial_available")
                  : t("console.serial_disabled")}
              </Tag>
            </div>
            <div className="vm-console-option">
              <Radio value="VNC" disabled={!vncAvailable}>
                <Space direction="vertical" size={2}>
                  <Text strong>{t("console.option_vnc_title")}</Text>
                  <Text type="secondary">
                    {t("console.option_vnc_description")}
                  </Text>
                </Space>
              </Radio>
              <Tag color={vncAvailable ? "blue" : "default"}>
                {vncAvailable
                  ? t("console.vnc_available")
                  : t("console.graphics_disabled")}
              </Tag>
            </div>
          </Space>
        </Radio.Group>
        {remoteAccessMode ? (
          <div className="vm-console-support-note">
            <Space direction="vertical" size={4} style={{ width: "100%" }}>
              <Text strong>{t("console.remote_access_note")}</Text>
              <Space wrap size={[8, 8]}>
                <Tag color={remoteAccessMode === "RDP" ? "blue" : "green"}>
                  {remoteAccessMode}
                </Tag>
                {remoteAccessCommand ? (
                  <Text
                    code={true}
                    copyable={{ text: remoteAccessCommand }}
                    className="selectable-inline-text"
                  >
                    {remoteAccessCommand}
                  </Text>
                ) : null}
              </Space>
              {remoteAccessDescription ? (
                <Text type="secondary">{remoteAccessDescription}</Text>
              ) : null}
            </Space>
          </div>
        ) : null}
        <Text type="secondary">{t("console.vnc_guest_output_hint")}</Text>
        <Text type="secondary">{t("console.chooser_default_hint")}</Text>
      </Space>
    </Modal>
  );
}
