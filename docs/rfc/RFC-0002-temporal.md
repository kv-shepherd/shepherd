# RFC-0002: Temporal Workflow Integration

> **Status**: Deferred  
> **Priority**: P2  
> **Source**: ADR-0005  
> **Trigger**: Multi-level approval or complex workflow orchestration required

---

## Problem

Current simple approval engine (approve/reject only) may not meet future requirements for:
- Multi-level approval (e.g., Team Lead → Manager → Director)
- Timeout auto-processing (e.g., auto-escalate after 24 hours)
- Complex workflow orchestration (conditional branches, parallel approval)
- Workflow visualization and replay capability

---

## Current State

**Not implementing**

| Factor | Analysis |
|--------|----------|
| **Current needs** | Only simple approve/reject, no multi-level requirements |
| **Complexity** | Temporal introduces additional infrastructure and learning curve |
| **Current approach** | Self-built lightweight approval engine (Ent + JSONB) meets needs |

---

## Interface Reserved

Current design reserves extension interface:

```go
// internal/governance/approval/dispatcher.go

type ApprovalDispatcher interface {
    Submit(ctx context.Context, ticket *ApprovalTicket) error
    Cancel(ctx context.Context, ticketID string) error
    GetStatus(ctx context.Context, ticketID string) (*ApprovalStatus, error)
}

// Current: LocalDispatcher (built-in engine)
// Future: TemporalDispatcher
```

---

## Implementation Path

### 1. Deploy Temporal Server

```yaml
services:
  temporal:
    image: temporalio/auto-setup:1.24
    ports:
      - "7233:7233"
    environment:
      - DB=postgresql
```

### 2. Implement TemporalDispatcher

```go
type TemporalDispatcher struct {
    client client.Client
}

func (d *TemporalDispatcher) Submit(ctx context.Context, ticket *ApprovalTicket) error {
    workflowOptions := client.StartWorkflowOptions{
        ID:        ticket.TicketID,
        TaskQueue: "approval-tasks",
    }
    _, err := d.client.ExecuteWorkflow(ctx, workflowOptions, ApprovalWorkflow, ticket)
    return err
}
```

### 3. Define Approval Workflow

```go
func ApprovalWorkflow(ctx workflow.Context, ticket *ApprovalTicket) error {
    // First level approval
    var firstApproval bool
    err := workflow.ExecuteActivity(ctx, WaitForApprovalActivity, ticket, "level1").Get(ctx, &firstApproval)
    if err != nil || !firstApproval {
        return errors.New("first level rejected")
    }
    
    // Second level (if required)
    if ticket.RequiresSecondLevel {
        // ...
    }
    
    return nil
}
```

---

## Feature Comparison

| Feature | Self-built Engine | Temporal |
|---------|-------------------|----------|
| Simple approve/reject | ✅ | ✅ |
| Multi-level approval | ❌ Self-implement | ✅ Native |
| Timeout auto-processing | ❌ Self-implement | ✅ Native |
| Workflow visualization | ❌ | ✅ Web UI |
| Operations complexity | Low | Medium |

---

## References

- [Temporal Documentation](https://docs.temporal.io/)
- [Temporal Go SDK](https://github.com/temporalio/sdk-go)
- [ADR-0005: Workflow Extensibility](../adr/ADR-0005-workflow-extensibility.md)
