# Transactions and Consistency

> **Purpose**: Define transaction boundaries and async consistency model for database writes.

---

## Governing ADR Constraints

- ADR-0006: unified async model for external-system writes
- ADR-0009: immutable DomainEvent payload (claim-check)
- ADR-0012: hybrid transaction strategy (Ent + sqlc scope control)
- ADR-0015: approval/governance workflow model

---

## Canonical Write Pattern

1. Synchronous transaction (request phase):
   - Validate business constraints.
   - Write core DB records (for example: ticket + domain event + initial audit).
   - Insert River job in-transaction when required by workflow.
2. Asynchronous worker phase:
   - Consume DomainEvent/Job.
   - Call provider/K8s API outside DB transaction.
   - Persist final state/audit updates.

---

## Hard Rules

- No K8s/provider calls inside database transactions.
- DomainEvent payload is append-only/immutable.
- Do not mix sqlc transaction handle and Ent transaction handle incorrectly.
- For delete flows:
  - Apply cascade checks before execution.
  - Use transient `DELETING` lock where defined.
  - Hard-delete primary row only after execution success.

---

## Failure Handling Baseline

| Failure Point | Expected Behavior |
|------|------|
| Validation/pre-check failure | Reject request, no side-effect writes |
| Transaction failure (request phase) | Rollback all writes in that transaction |
| Worker external call failure | Keep record in retryable/failed state, append failure audit |
| Partial batch failure | Preserve successful children, expose aggregate `PARTIAL_SUCCESS` |

---

## References

- [03-service-layer.md §3 Transaction Integration](../phases/03-service-layer.md#3-transaction-integration-adr-0012)
- [04-governance.md §2 River Queue](../phases/04-governance.md#2-river-queue-adr-0006)
- [04-governance.md §3 Domain Event Pattern](../phases/04-governance.md#3-domain-event-pattern-adr-0009)
- [ADR-0006 §Decision](../../adr/ADR-0006-unified-async-model.md#decision)
- [ADR-0012 §Decision](../../adr/ADR-0012-hybrid-transaction.md#decision)
