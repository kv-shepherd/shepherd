# RFC-0018: External Notification Channels

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: V1 completed; user/enterprise request for Email/Webhook/Slack notifications  
> **Related**: [ADR-0015 §20](../adr/ADR-0015-governance-model-v2.md), [04-governance.md §6.3](../design/phases/04-governance.md#63-notification-system-adr-0015-20)

---

## Problem

V1 implements platform-internal inbox only (PostgreSQL-backed). Users have no way to receive real-time notifications outside the platform UI:

| Limitation | Impact |
|------------|--------|
| No email notifications | Users must repeatedly check the platform for updates |
| No webhook integration | Cannot trigger external automation (ChatOps, ServiceNow) |
| No mobile push | No real-time alerts for critical events |

Enterprise environments typically require integration with existing communication infrastructure (corporate email, Slack/Teams, PagerDuty-style alerting).

---

## V1 Foundation

V1 establishes the foundation for external notifications:

```go
// V1: Decoupled NotificationSender interface
type NotificationSender interface {
    Send(ctx context.Context, notification *Notification) error
    SendBatch(ctx context.Context, notifications []*Notification) error
}

// V1: InboxSender (stores to PostgreSQL)
type InboxSender struct {
    db *ent.Client
}
```

> **Key Design Decision (ADR-0006 Compliance)**:
> 
> - **Internal Inbox** (V1): Synchronous writes within business transaction
> - **External Channels** (V2+): Async via River Queue for retry resilience

---

## Proposed Solution

### Channel Architecture

```
                              ┌─────────────────────┐
                              │  NotificationRouter │
                              │  (Fanout to channels)│
                              └─────────┬───────────┘
                                        │
          ┌─────────────────────────────┼─────────────────────────────┐
          │                             │                             │
          ▼                             ▼                             ▼
┌──────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│   InboxSender    │      │   EmailSender    │      │  WebhookSender   │
│ (Sync, same TX)  │      │ (Async, River)   │      │ (Async, River)   │
└──────────────────┘      └──────────────────┘      └──────────────────┘
        │                         │                         │
        ▼                         ▼                         ▼
   PostgreSQL               SMTP Server             External System
  (notifications)         (via template)          (JSON payload)
```

### River Queue Integration

External channel sends use River Queue for:
- **Retry on failure**: SMTP server down, webhook timeout
- **Rate limiting**: Prevent email spam, respect external API limits
- **Observability**: Track delivery status and failure reasons

```go
// internal/jobs/external_notification_job.go
type ExternalNotificationJobArgs struct {
    NotificationID string `json:"notification_id"`
    Channel        string `json:"channel"` // "email", "webhook", "slack"
    Attempt        int    `json:"attempt"`
}

func (ExternalNotificationJobArgs) Kind() string {
    return "external_notification_job"
}

// Worker with retry logic
func (w *ExternalNotificationWorker) Work(ctx context.Context, job *river.Job[ExternalNotificationJobArgs]) error {
    notification, err := w.notificationRepo.Get(ctx, job.Args.NotificationID)
    if err != nil {
        return river.JobCancel(err) // Not found, don't retry
    }
    
    sender := w.getSender(job.Args.Channel)
    if err := sender.Send(ctx, notification); err != nil {
        if isRetryable(err) {
            return err // River will retry
        }
        return river.JobCancel(err) // Non-retryable, mark failed
    }
    
    return nil
}
```

### Supported Channels

| Channel | Priority | Configuration | Use Case |
|---------|----------|---------------|----------|
| **Email** | P1 | SMTP settings | Approval notifications to team |
| **Webhook** | P1 | URL + secret | ChatOps, ServiceNow integration |
| **Slack** | P2 | Webhook URL or OAuth | Team channels |
| **Microsoft Teams** | P3 | Incoming Webhook | Enterprise Teams integration |
| **PagerDuty** | P3 | API integration | On-call alerting |

### User Preferences

Users can configure their notification preferences:

```go
// ent/schema/notification_preference.go
func (NotificationPreference) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("user_id").NotEmpty(),
        field.Enum("channel").Values("inbox", "email", "slack", "webhook"),
        field.Bool("enabled").Default(true),
        field.Strings("event_types").Optional(), // Filter which events
        field.JSON("config", map[string]interface{}{}), // Channel-specific config
    }
}
```

### Admin Configuration

Platform-wide notification settings:

| Setting | Description |
|---------|-------------|
| Default channels | Which channels are enabled by default |
| Email templates | Customizable email templates per event type |
| Webhook retry policy | Max attempts, backoff strategy |
| Rate limits | Max emails per user per hour |

---

## Trade-offs

### Pros

- **User experience**: Real-time notifications without platform polling
- **Enterprise integration**: Works with existing communication tools
- **Automation**: Webhook enables ChatOps and external workflows
- **River Queue**: Built on existing async infrastructure

### Cons

- **Complexity**: Additional configuration and maintenance burden
- **External dependencies**: SMTP server, Slack API availability
- **Security**: Webhook secrets, email content sensitivity
- **Cost**: External service usage (Slack API limits, email delivery services)

---

## Implementation Notes

### Phase 1: Email + Webhook (P1)

1. Add `NotificationRouter` to fanout to multiple senders
2. Implement `EmailSender` with SMTP configuration
3. Implement `WebhookSender` with HMAC signature
4. Add River job for async external notification
5. Create admin UI for channel configuration

### Phase 2: Rich Integrations (P2+)

1. Native Slack integration (OAuth flow)
2. Microsoft Teams connector
3. User preference UI
4. Notification templating engine

### Security Considerations

| Concern | Mitigation |
|---------|------------|
| Email content exposure | Use templates, redact sensitive data |
| Webhook secret management | Encrypt at rest (ADR-0019) |
| Rate limiting | Per-channel, per-user limits |
| Audit logging | Log all external notification attempts |

---

## Migration Path

No breaking changes. V1 inbox continues to work. External channels are additive:

```
V1: InboxSender only (synchronous)
V2: InboxSender + EmailSender + WebhookSender (parallel fanout)
```

---

## References

- [ADR-0015 §20 Notification System](../adr/ADR-0015-governance-model-v2.md)
- [ADR-0006 Unified Async Model](../adr/ADR-0006-unified-async-model.md)
- [04-governance.md §6.3](../design/phases/04-governance.md#63-notification-system-adr-0015-20)
- [River Queue Documentation](https://riverqueue.com/docs)
