"use client";

type ApprovalPlacementVolumeMode = "Block" | "Filesystem";

interface ApprovalClusterPlacementMemory {
  version: 1;
  clusterId: string;
  selectedStorageClass?: string;
  selectedRootVolumeModeKey?: string;
  selectedDVAccessModes?: string[];
  selectedDVVolumeMode?: ApprovalPlacementVolumeMode;
  updatedAt: string;
}

const APPROVAL_CLUSTER_PLACEMENT_STORAGE_PREFIX =
  "shepherd-approval-cluster-placement";

function normalizeOptionalString(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function normalizeVolumeMode(
  value: unknown,
): ApprovalPlacementVolumeMode | undefined {
  return value === "Block" || value === "Filesystem" ? value : undefined;
}

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const normalized = value
    .map((item) => (typeof item === "string" ? item.trim() : ""))
    .filter((item) => item !== "");
  return Array.from(new Set(normalized)).sort();
}

function buildApprovalClusterPlacementStorageKey(clusterId: string): string {
  return `${APPROVAL_CLUSTER_PLACEMENT_STORAGE_PREFIX}:${clusterId}`;
}

function hasMeaningfulPlacement(
  placement: Partial<ApprovalClusterPlacementMemory>,
): boolean {
  return Boolean(
    placement.selectedStorageClass ||
      placement.selectedRootVolumeModeKey ||
      (placement.selectedDVAccessModes &&
        placement.selectedDVAccessModes.length > 0 &&
        placement.selectedDVVolumeMode),
  );
}

export function loadRememberedApprovalClusterPlacement(
  clusterId: string,
): ApprovalClusterPlacementMemory | null {
  const normalizedClusterId = normalizeOptionalString(clusterId);
  if (typeof window === "undefined" || !normalizedClusterId) {
    return null;
  }

  try {
    const raw = window.localStorage.getItem(
      buildApprovalClusterPlacementStorageKey(normalizedClusterId),
    );
    if (!raw) {
      return null;
    }

    const parsed = JSON.parse(raw) as Partial<ApprovalClusterPlacementMemory>;
    const remembered: ApprovalClusterPlacementMemory = {
      version: 1,
      clusterId: normalizedClusterId,
      selectedStorageClass: normalizeOptionalString(parsed.selectedStorageClass),
      selectedRootVolumeModeKey: normalizeOptionalString(
        parsed.selectedRootVolumeModeKey,
      ),
      selectedDVAccessModes: normalizeStringArray(parsed.selectedDVAccessModes),
      selectedDVVolumeMode: normalizeVolumeMode(parsed.selectedDVVolumeMode),
      updatedAt:
        normalizeOptionalString(parsed.updatedAt) ?? new Date(0).toISOString(),
    };

    if (!hasMeaningfulPlacement(remembered)) {
      window.localStorage.removeItem(
        buildApprovalClusterPlacementStorageKey(normalizedClusterId),
      );
      return null;
    }

    return remembered;
  } catch {
    window.localStorage.removeItem(
      buildApprovalClusterPlacementStorageKey(normalizedClusterId),
    );
    return null;
  }
}

export function saveRememberedApprovalClusterPlacement(input: {
  clusterId: string;
  selectedStorageClass?: string;
  selectedRootVolumeModeKey?: string;
  selectedDVAccessModes?: string[];
  selectedDVVolumeMode?: ApprovalPlacementVolumeMode;
}) {
  const clusterId = normalizeOptionalString(input.clusterId);
  if (typeof window === "undefined" || !clusterId) {
    return;
  }

  const remembered: ApprovalClusterPlacementMemory = {
    version: 1,
    clusterId,
    selectedStorageClass: normalizeOptionalString(input.selectedStorageClass),
    selectedRootVolumeModeKey: normalizeOptionalString(
      input.selectedRootVolumeModeKey,
    ),
    selectedDVAccessModes: normalizeStringArray(input.selectedDVAccessModes),
    selectedDVVolumeMode: normalizeVolumeMode(input.selectedDVVolumeMode),
    updatedAt: new Date().toISOString(),
  };

  if (!hasMeaningfulPlacement(remembered)) {
    window.localStorage.removeItem(
      buildApprovalClusterPlacementStorageKey(clusterId),
    );
    return;
  }

  try {
    window.localStorage.setItem(
      buildApprovalClusterPlacementStorageKey(clusterId),
      JSON.stringify(remembered),
    );
  } catch {
    // Ignore storage failures and keep approval flows working.
  }
}
