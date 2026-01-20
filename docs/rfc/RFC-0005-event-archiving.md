# RFC-0005: Physical Event Archiving

> **Status**: Deferred  
> **Priority**: P2  
> **Source**: ADR-0009  
> **Trigger**: DomainEvent table size becomes too large

---

## Problem

As the platform operates over time, the DomainEvent table accumulates historical records. While soft archiving (`archived_at` field) handles logical separation, physical archiving may be needed for storage efficiency.

---

## Current State

V1.0 uses soft archiving via `archived_at` field with daily River periodic job to mark old events.

---

## Proposed Solution

### Physical Archiving Strategy

```sql
-- Move archived events to separate archive table
INSERT INTO domain_events_archive
SELECT * FROM domain_events
WHERE archived_at IS NOT NULL
AND archived_at < NOW() - INTERVAL '90 days';

DELETE FROM domain_events
WHERE archived_at IS NOT NULL
AND archived_at < NOW() - INTERVAL '90 days';
```

### Retention Policy

| Status | Soft Archive | Physical Archive | Delete |
|--------|--------------|------------------|--------|
| COMPLETED | 30 days | 90 days | 365 days |
| FAILED | 90 days | 180 days | 730 days |
| CANCELLED | 7 days | 30 days | 90 days |

---

## Trigger Conditions

- DomainEvent table > 10 million rows
- Query performance degradation on event queries
- Storage capacity concerns

---

## References

- [ADR-0009: Domain Event Pattern](../adr/ADR-0009-domain-event-pattern.md)
