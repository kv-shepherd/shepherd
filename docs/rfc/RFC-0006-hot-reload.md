# RFC-0006: Configuration Admin API (Hot Reload)

> **Status**: Deferred  
> **Priority**: P2  
> **Source**: Phase 00  
> **Trigger**: Dynamic runtime configuration via API required

---

## Problem

Current runtime configuration changes are intentionally narrow and process-local.
Future requirements may include:
- Runtime configuration changes via REST API
- Multi-instance configuration synchronization
- Configuration change audit trail

---

## Current State

V1.0 only has limited runtime hot-reload primitives today, most notably zap
`AtomicLevel` for dynamic log-level changes. It does **not** yet provide a
general configuration admin API or cross-instance synchronization. This RFC is
therefore about a broader operator-facing config-management capability, not a
description of shipped V1 behavior.

---

## Proposed Solution

### Admin API Endpoints

```
GET  /api/admin/config
PUT  /api/admin/config
POST /api/admin/config/reload
```

### Config Change Broadcast

Option A: Redis Pub/Sub
```go
// On config change, publish to channel
redis.Publish(ctx, "config:changed", configVersion)
```

Option B: PostgreSQL LISTEN/NOTIFY
```sql
NOTIFY config_changed, 'v1.2.3';
```

---

## Trigger Conditions

- Operations team requests API-based config management
- Multi-instance deployment needs synchronized config
- Compliance requires config change audit

---

## References

- [Phase 00: Prerequisites](../design/phases/00-prerequisites.md)
