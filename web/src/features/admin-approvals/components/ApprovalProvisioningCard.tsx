import { Alert, Card, Descriptions, Space, Tag } from "antd";
import { useTranslation } from "react-i18next";

import type { ApprovalTask } from "../types";

export function hasProvisioningFailure(
  provisioning: NonNullable<ApprovalTask["provisioning"]>,
): boolean {
  return (
    Boolean(provisioning.failure_message?.trim()) &&
    provisioning.phase?.trim().toLowerCase() === "failed"
  );
}

export function getProvisioningPhaseTagColor(phase?: string): string {
  if (!phase) return "default";
  if (phase === "Succeeded" || phase === "Ready") return "green";
  if (phase === "Failed") return "red";
  return "blue";
}

export function getCloneTypeTagColor(cloneType?: string): string {
  if (!cloneType) return "default";
  if (cloneType === "copy") return "orange";
  return "geekblue";
}

type ApprovalProvisioningCardProps = {
  provisioning: NonNullable<ApprovalTask["provisioning"]>;
};

export function ApprovalProvisioningCard({
  provisioning,
}: ApprovalProvisioningCardProps) {
  const { t } = useTranslation(["approval", "common"]);

  return (
    <Card
      size="small"
      title={t("approve_modal.provisioning.title", "Provisioning Status")}
      extra={
        <Tag
          color={getProvisioningPhaseTagColor(provisioning.phase)}
          data-testid="approval-provisioning-phase"
        >
          {provisioning.phase || "—"}
        </Tag>
      }
      style={{ marginBottom: 16 }}
      data-testid="approval-provisioning-card"
    >
      <Space direction="vertical" size={12} style={{ width: "100%" }}>
        <Descriptions
          bordered
          size="small"
          column={2}
          items={[
            {
              key: "progress",
              label: t("approve_modal.provisioning.progress", "Progress"),
              children: provisioning.progress || "—",
            },
            {
              key: "rootClaim",
              label: t("approve_modal.provisioning.root_claim", "Root Claim"),
              children: provisioning.claim_name || "—",
            },
            {
              key: "pvcPhase",
              label: t("approve_modal.provisioning.pvc_phase", "PVC Phase"),
              children: provisioning.pvc_phase || "—",
            },
            {
              key: "cloneType",
              label: t("approve_modal.provisioning.clone_type", "Clone Type"),
              children: provisioning.clone_type ? (
                <Tag
                  color={getCloneTypeTagColor(provisioning.clone_type)}
                  data-testid="approval-provisioning-clone-type"
                >
                  {provisioning.clone_type === "copy"
                    ? t(
                        "approve_modal.provisioning.clone_type_copy",
                        "Host-assisted copy",
                      )
                    : provisioning.clone_type}
                </Tag>
              ) : (
                "—"
              ),
            },
            {
              key: "clonePhase",
              label: t("approve_modal.provisioning.clone_phase", "Clone Phase"),
              children: provisioning.clone_phase || "—",
            },
          ]}
        />
        {provisioning.clone_fallback_reason && (
          <Alert
            type="warning"
            showIcon
            message={t(
              "approve_modal.provisioning.clone_fallback_reason",
              "Clone fallback reason",
            )}
            description={provisioning.clone_fallback_reason}
          />
        )}
        {hasProvisioningFailure(provisioning) && (
          <Alert
            type="error"
            showIcon
            message={t(
              "approve_modal.provisioning.failure_message",
              "Provisioning failure",
            )}
            description={provisioning.failure_message}
          />
        )}
      </Space>
    </Card>
  );
}
