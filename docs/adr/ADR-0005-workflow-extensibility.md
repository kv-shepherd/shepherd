# ADR-0005: Workflow Extensibility Design

> **Status**: Accepted  
> **Date**: 2026-01-14

---

## Context

The governance platform requires approval workflows to manage sensitive operations like VM creation and deletion.

### Core Requirements Analysis

| Requirement | Priority | This Release |
|-------------|----------|--------------|
| Approve/Reject | P0 Required | ✅ |
| Admin can modify parameters during approval | P0 Required | ✅ |
| Multi-level approval | P2 Optional | ❌ Roadmap |
| Withdraw/Countersign/Transfer | P3 Not needed | ❌ Not implementing |
| Timeout auto-processing | P2 Optional | ❌ Roadmap |

> **Design Principle**:
> 
> Keep approval workflow **extremely simple**, only supporting approve/reject. Not implementing withdraw, countersign, transfer, auto-timeout rejection, or other complex features.
> A self-built state machine becomes a maintenance nightmare once business complexity increases—this is Phase 4's boundary red line.

---

## Decision

### Adopt: Simplified Approval Flow + Admin Can Modify Parameters

**Core Features**:

1. **Minimal states**: Only `PENDING` → `APPROVED` / `REJECTED` two paths
2. **Admin can modify parameters**: Directly adjust user-submitted configuration during approval (CPU/memory/image), reducing back-and-forth communication

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Approval Flow (Simplified)                       │
│                                                                      │
│  User submits request                                                │
│       │                                                              │
│       ▼                                                              │
│   ┌─────────┐                                                        │
│   │ PENDING │ ← Admin can view and **modify** user-submitted params  │
│   └────┬────┘                                                        │
│        │                                                             │
│   ┌────┴────┐                                                        │
│   │         │                                                        │
│   ▼         ▼                                                        │
│ APPROVED  REJECTED                                                   │
│ (execute) (end)                                                      │
│                                                                      │
│  📌 If admin modifies parameters:                                    │
│     - modified_spec stores the modified configuration                │
│     - modification_reason records the reason                         │
│     - Actual execution uses modified_spec (if present)               │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Data Model

```go
// ApprovalTicket core fields
type ApprovalTicket struct {
    TicketID           string                 // Ticket ID
    Status             string                 // PENDING, APPROVED, REJECTED
    Spec               map[string]interface{} // User's original submitted parameters
    ModifiedSpec       map[string]interface{} // Admin's modified parameters (optional)
    ModificationReason string                 // Modification reason (optional)
    DecidedBy          string                 // Approver
    DecisionReason     string                 // Approval comments
}

// GetEffectiveSpec returns the configuration to actually execute
func (t *ApprovalTicket) GetEffectiveSpec() map[string]interface{} {
    if len(t.ModifiedSpec) > 0 {
        return t.ModifiedSpec // Use admin's modified configuration
    }
    return t.Spec // Use user's original configuration
}
```

---

## Interface Extensibility

```go
type WorkflowExecutor interface {
    Start(ctx, ticketID, payload) error
    Signal(ctx, ticketID, action string, data) error  // action: "approve", "reject"
    Query(ctx, ticketID) (*WorkflowState, error)
    Cancel(ctx, ticketID, reason) error               // Reserved, not currently used
}
```

---

## Consequences

### Positive

- ✅ Simple state machine, easy to maintain
- ✅ Admin can adjust parameters without rejection-resubmit cycle
- ✅ Clear audit trail (original vs modified specs preserved)
- ✅ Interface reserved for future workflow engine integration

### Negative

- 🟡 No multi-level approval in v1.0
- 🟡 No auto-timeout handling in v1.0

### Mitigation

- Multi-level approval and auto-timeout planned for Roadmap (RFC-0002)
- Current simple workflow covers 90%+ of use cases

---

## Temporal Integration (Roadmap)

> **Roadmap**: See [RFC-0002 Temporal Workflow Integration](../rfc/RFC-0002-temporal.md)
>
> **Trigger**: When multi-level approval or timeout auto-processing is required.

---

## References

- [Temporal Go SDK](https://docs.temporal.io/dev-guide/go)
- [State Machines vs Workflows](https://temporal.io/blog/state-machines-vs-workflows)
