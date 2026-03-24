import { describe, expect, it } from "vitest";

import { buildApprovalBatchDisplayItems } from "./summary";

const t = ((key: string, options?: { index?: number }) => {
  if (key === "summary.item_fallback" && typeof options?.index === "number") {
    return `Request #${options.index}`;
  }
  if (key.startsWith("summary.shape_")) {
    return key;
  }
  if (key.startsWith("vm:status.")) {
    return key;
  }
  if (key.startsWith("summary.power_action.")) {
    return key;
  }
  return key;
}) as never;

describe("buildApprovalBatchDisplayItems", () => {
  it("builds unique keys when summary items reuse the same vm id", () => {
    const items = buildApprovalBatchDisplayItems(
      {
        id: "ticket-1",
        status: "PENDING",
        requester: "alice",
        summary: {
          items: [
            {
              vm_id: "019d19d9-7c61-71a3-a5ed-861c42d4695b",
              vm_name: "vm-a",
              system_id: "sys-1",
              service_id: "svc-1",
              namespace: "prod-a",
            },
            {
              vm_id: "019d19d9-7c61-71a3-a5ed-861c42d4695b",
              vm_name: "vm-a",
              system_id: "sys-1",
              service_id: "svc-1",
              namespace: "prod-a",
            },
          ],
        },
      },
      t,
    );

    expect(items).toHaveLength(2);
    expect(items[0]?.key).not.toBe(items[1]?.key);
  });
});
