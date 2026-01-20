# RFC-0006: Configuration Admin API (Hot Reload)

> **Status**: Deferred  
> **Priority**: P2  
> **Source**: Phase 00  
> **Trigger**: Dynamic runtime configuration via API required

---

## Problem

Current configuration hot-reload uses file-based `fsnotify`. Future requirements may include:
- Runtime configuration changes via REST API
- Multi-instance configuration synchronization
- Configuration change audit trail

---

## Current State

V1.0 uses file-based hot-reload with fsnotify watching config files.

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

- [Phase 00: Prerequisites](../projects/core-go/phases/00-prerequisites.md)
