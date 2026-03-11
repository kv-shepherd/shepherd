import { Alert, Card, Descriptions, Tag } from "antd";
import { useTranslation } from "react-i18next";

import type { ApprovalTicket } from "../types";

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
  provisioning: NonNullable<ApprovalTicket["provisioning"]>;
};

export function ApprovalProvisioningCard({
  provisioning,
}: ApprovalProvisioningCardProps) {
  const { t } = useTranslation(["approval", "common"]);

  return (
    <Card
      size="small"
      title={t("approve_modal.provisioning.title", "Provisioning Status")}
      style={{ marginBottom: 16 }}
      data-testid="approval-provisioning-card"
    >
      <Descriptions bordered size="small" column={1}>
        <Descriptions.Item
          label={t("approve_modal.provisioning.phase", "Phase")}
        >
          <Tag
            color={getProvisioningPhaseTagColor(provisioning.phase)}
            data-testid="approval-provisioning-phase"
          >
            {provisioning.phase || "—"}
          </Tag>
        </Descriptions.Item>
        <Descriptions.Item
          label={t("approve_modal.provisioning.progress", "Progress")}
        >
          {provisioning.progress || "—"}
        </Descriptions.Item>
        <Descriptions.Item
          label={t("approve_modal.provisioning.root_claim", "Root Claim")}
        >
          {provisioning.claim_name || "—"}
        </Descriptions.Item>
        <Descriptions.Item
          label={t("approve_modal.provisioning.pvc_phase", "PVC Phase")}
        >
          {provisioning.pvc_phase || "—"}
        </Descriptions.Item>
        <Descriptions.Item
          label={t("approve_modal.provisioning.clone_type", "Clone Type")}
        >
          {provisioning.clone_type ? (
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
          )}
        </Descriptions.Item>
        <Descriptions.Item
          label={t("approve_modal.provisioning.clone_phase", "Clone Phase")}
        >
          {provisioning.clone_phase || "—"}
        </Descriptions.Item>
      </Descriptions>
      {provisioning.clone_fallback_reason && (
        <Alert
          type="warning"
          showIcon
          style={{ marginTop: 12 }}
          message={t(
            "approve_modal.provisioning.clone_fallback_reason",
            "Clone fallback reason",
          )}
          description={provisioning.clone_fallback_reason}
        />
      )}
      {provisioning.failure_message && (
        <Alert
          type="error"
          showIcon
          style={{ marginTop: 12 }}
          message={t(
            "approve_modal.provisioning.failure_message",
            "Provisioning failure",
          )}
          description={provisioning.failure_message}
        />
      )}
    </Card>
  );
}
