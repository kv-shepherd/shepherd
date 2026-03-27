import type { TFunction } from "i18next";

import type { VM } from "@/features/vm-management/types";

type VMRemoteAccessMode = "SSH" | "RDP";

const normalize = (value?: string | null): string => value?.trim() ?? "";

const containsIgnoreCase = (text: string, fragment: string): boolean =>
  text.toLowerCase().includes(fragment.toLowerCase());

export const formatVMOperatingSystem = (vm?: Partial<VM> | null): string => {
  const osName = normalize(vm?.os_name);
  const osVersion = normalize(vm?.os_version);
  const osFamily = normalize(vm?.os_family);
  const familyLabel =
    osFamily === "linux"
      ? "Linux"
      : osFamily === "windows"
        ? "Windows"
        : osFamily;

  if (osName && osVersion) {
    if (containsIgnoreCase(osName, osVersion)) {
      return osName;
    }
    return `${osName} ${osVersion}`.trim();
  }
  if (osName) {
    return osName;
  }
  if (familyLabel && osVersion) {
    return `${familyLabel} ${osVersion}`.trim();
  }
  return familyLabel || "";
};

export const resolveVMRemoteAccessMode = (
  vm?: Partial<VM> | null,
): VMRemoteAccessMode | null => {
  const ipAddress = normalize(vm?.ip_address);
  if (!ipAddress) {
    return null;
  }
  const osFamily = normalize(vm?.os_family).toLowerCase();
  const osName = normalize(vm?.os_name).toLowerCase();
  if (osFamily === "windows" || containsIgnoreCase(osName, "windows")) {
    return "RDP";
  }
  return "SSH";
};

export const buildVMRemoteAccessCommand = (
  vm?: Partial<VM> | null,
): string | null => {
  const ipAddress = normalize(vm?.ip_address);
  if (!ipAddress) {
    return null;
  }
  const mode = resolveVMRemoteAccessMode(vm);
  if (mode === "RDP") {
    return `mstsc /v:${ipAddress}`;
  }
  if (mode === "SSH") {
    return `ssh <username>@${ipAddress}`;
  }
  return null;
};

export const describeVMRemoteAccess = (
  t: TFunction,
  vm?: Partial<VM> | null,
): string | null => {
  const mode = resolveVMRemoteAccessMode(vm);
  if (mode === "RDP") {
    return t("remote_access.rdp_help");
  }
  if (mode === "SSH") {
    return t("remote_access.ssh_help");
  }
  return null;
};
