# RFC-0004: External Approval Systems Integration

> **Status**: Proposed  
> **Priority**: P1 (V1+)  
> **Source**: ADR-0005  
> **Design**: [Master Flow Stage 2.E](../design/interaction-flows/master-flow.md#stage-2-e)  
> **Review Period**: Until 2026-02-01 (48-hour minimum)  
> **Discussion**: [Issue #58](https://github.com/kv-shepherd/shepherd/issues/58)

---

## Problem

Enterprise environments often require integration with existing approval systems (ServiceNow, JIRA, internal OA systems) rather than using only the built-in approval engine. This is critical for:
- Compliance with existing enterprise governance workflows
- Audit trail integration with centralized logging systems
- Single pane of glass for all IT approvals

---

## Decision

Implement a **pluggable external approval system** with the following characteristics:

### Core Principles (aligned with industry best practices)

| Principle | Implementation |
|-----------|---------------|
| **Fail-safe default** | Built-in approval if external system unavailable |
| **Secure communication** | TLS mandatory, HMAC signature verification |
| **Audit completeness** | All external decisions logged locally |
| **Timeout handling** | Configurable timeout with fallback behavior |
| **Retry with backoff** | Exponential backoff for transient failures |

### Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                     External Approval Integration                                │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌─────────────┐     Webhook      ┌──────────────┐     Callback    ┌──────────┐│
│  │   Shepherd  │ ───────────────► │ External Sys │ ──────────────► │ Shepherd ││
│  │ (Initiator) │   (TLS + HMAC)   │(ServiceNow)  │  (Signed JWT)   │(Receiver)││
│  └─────────────┘                  └──────────────┘                  └──────────┘│
│        │                                                                  │      │
│        │                      ┌───────────────┐                          │      │
│        └──────────────────────│ Audit Logs    │◄─────────────────────────┘      │
│                               │ (Local Copy)  │                                  │
│                               └───────────────┘                                  │
│                                                                                  │
│  Fallback: If external system is unavailable → fall back to built-in approval   │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation

### Webhook Mode (Push)

Platform pushes approval requests to external system.

```go
// WebhookApprovalHandler implements external approval via outbound webhook
type WebhookApprovalHandler struct {
    webhookURL     string
    secret         string            // HMAC signing key
    timeout        time.Duration     // Default: 30s
    retryCount     int               // Default: 3
    retryBackoff   time.Duration     // Default: 2s (exponential)
    httpClient     *http.Client
}

func (h *WebhookApprovalHandler) Submit(ctx context.Context, ticket *ApprovalTicket) error {
    payload := buildWebhookPayload(ticket)
    signature := hmacSign(payload, h.secret)
    
    req, _ := http.NewRequestWithContext(ctx, "POST", h.webhookURL, payload)
    req.Header.Set("X-Signature-256", signature)
    req.Header.Set("X-Ticket-ID", ticket.ID)
    
    return h.sendWithRetry(req)
}
```

### Callback Endpoint (Receive Decision)

```go
// POST /api/v1/webhooks/approval-callback
func (h *WebhookCallbackHandler) HandleCallback(c *gin.Context) {
    // 1. Verify signature (HMAC-SHA256)
    if !verifySignature(c.Request, h.secret) {
        c.JSON(401, gin.H{"error": "invalid signature"})
        return
    }
    
    // 2. Parse decision
    var decision ExternalDecision
    if err := c.ShouldBindJSON(&decision); err != nil {
        c.JSON(400, gin.H{"error": "invalid payload"})
        return
    }
    
    // 3. Apply decision atomically
    err := h.approvalService.ApplyExternalDecision(c.Request.Context(), decision)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{"status": "accepted"})
}
```

### Polling Mode (Pull)

External system pulls pending approvals via API.

```
GET /api/v1/approvals/pending?external_system=servicenow
→ Returns list of tickets awaiting external decision

POST /api/v1/approvals/{id}/decision
→ Submit decision with signature verification
```

---

## Security Requirements

| Requirement | Implementation |
|-------------|---------------|
| **Transport Security** | TLS 1.2+ mandatory for all webhook traffic |
| **Request Signing** | HMAC-SHA256 signature in `X-Signature-256` header |
| **Callback Verification** | JWT with short expiry (5 min) for callback authentication |
| **Secret Storage** | AES-256-GCM encryption at rest |
| **Audit Logging** | All external calls logged (redact secrets per ADR-0019) |

---

## Schema

> Defined in [Master Flow Stage 2.E](../design/interaction-flows/master-flow.md#stage-2-e)

```sql
CREATE TABLE external_approval_systems (
    id              VARCHAR(36) PRIMARY KEY,
    name            VARCHAR(255) NOT NULL UNIQUE,
    type            VARCHAR(50) NOT NULL,  -- 'webhook', 'servicenow', 'jira'
    enabled         BOOLEAN DEFAULT true,
    webhook_url     TEXT,
    webhook_secret  TEXT,                   -- Encrypted (AES-256-GCM)
    webhook_headers JSONB,                  -- Custom headers
    timeout_seconds INTEGER DEFAULT 30,
    retry_count     INTEGER DEFAULT 3,
    created_by      VARCHAR(255) NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Fallback Strategy

| Scenario | Behavior |
|----------|----------|
| External system timeout | Retry with exponential backoff |
| All retries exhausted | Fall back to built-in approval queue |
| External system returns error | Log error, fall back to built-in |
| Invalid callback signature | Reject, log security event |

---

## Monitoring & Observability

| Metric | Purpose |
|--------|---------|
| `external_approval_requests_total` | Total webhook requests sent |
| `external_approval_latency_seconds` | Webhook response time |
| `external_approval_errors_total` | Failed webhook calls |
| `external_approval_fallback_total` | Fallback to built-in count |

---

## References

- [ADR-0005: Workflow Extensibility](../adr/ADR-0005-workflow-extensibility.md)
- [Master Flow Stage 2.E](../design/interaction-flows/master-flow.md#stage-2-e)
- [ADR-0019: Governance Security Baseline](../adr/ADR-0019-governance-security-baseline-controls.md) (Audit redaction)

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-14 | @agent | Initial RFC (Deferred) |
| 2026-01-29 | @jindyzhao | Promoted to Accepted; added security and fallback details |
