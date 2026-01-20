# RFC-0004: External Approval Systems

> **Status**: Deferred  
> **Priority**: P2  
> **Source**: ADR-0005  
> **Trigger**: Integration with external workflow systems (ServiceNow, JIRA, etc.)

---

## Problem

Enterprise environments may require integration with existing approval systems rather than using the built-in approval engine.

---

## Proposed Approaches

### Webhook Mode (Push)

Platform pushes approval requests to external system.

```go
type WebhookApprovalHandler struct {
    webhookURL string
    secret     string
}

func (h *WebhookApprovalHandler) Submit(ctx context.Context, ticket *ApprovalTicket) error {
    payload := buildWebhookPayload(ticket)
    return h.sendWebhook(ctx, payload)
}
```

### Polling Mode (Pull)

External system pulls pending approvals via API.

```
GET /api/v1/approvals/pending?external_system=servicenow
POST /api/v1/approvals/{id}/decision
```

---

## Trigger Conditions

- Enterprise requires ServiceNow/JIRA integration
- Existing approval workflow must be preserved
- Compliance requires external audit trail

---

## References

- [ADR-0005: Workflow Extensibility](../adr/ADR-0005-workflow-extensibility.md)
