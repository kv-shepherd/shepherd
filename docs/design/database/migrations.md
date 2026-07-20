# Migrations

> **Purpose**: Define schema evolution workflow for business tables and queue tables.

---

## Tooling Ownership

| Scope | Tool | Source |
|------|------|------|
| Business schema | Atlas (from Ent schema) | `ent/schema/*.go`, Atlas migration files |
| Queue schema | River migration tool | `river migrate-up` managed tables |

---

## Apply Order

1. Apply Atlas migrations (business/domain tables).
2. Apply River migrations (queue runtime tables).
3. Run startup/schema validation checks.

Rationale:

- Business schema must exist before request/worker workflows rely on it.
- River tables are runtime dependencies for async processing.

---

## Startup Migration Modes

Production startup owns the default migration flow. Released Docker images and
Go runtime archives default `DATABASE_AUTO_APPLY_VERSIONED_MIGRATIONS=true`, so
the server inspects database state before accepting traffic:

| Database state | Startup behavior |
|------|------|
| Fresh database, no core tables, no `atlas_schema_revisions` | Bootstrap the current Ent schema, record the latest Atlas migration version as the baseline, then apply River migrations. |
| Existing schema without `atlas_schema_revisions` | Adopt the schema with Atlas versioned migrations using the dirty-database allowance, then apply River migrations. |
| Atlas-managed schema with `atlas_schema_revisions` | Apply pending Atlas migrations normally, then apply River migrations. |

Do not run raw `atlas migrate apply` against a fresh database as the first
operation. The Atlas directory contains reviewed incremental migrations, not a
full base-schema dump. Fresh deployments should use the server startup path so
the base schema and Atlas revision baseline are established together.
Release artifacts bundle the Atlas executable and migration directory needed by
this path.

PgBouncer dual-pool deployments must set `DATABASE_WORKER_HOST` (and
`DATABASE_WORKER_PORT` when it differs from the primary port) to one trusted,
session-capable endpoint. That endpoint is a deployment contract: it must route
to the same PostgreSQL cluster, database, and role as `DATABASE_URL`; it must not
be a replica, a different tenant/database, or a transaction-pool endpoint. The
server uses it for both the outer startup advisory lock and Atlas `migrate
apply`/`migrate set`, because Atlas also holds a session-level advisory lock.
The primary transaction-pool endpoint remains the application endpoint for
ordinary schema inspection and runtime queries. `DATABASE_WORKER_HOST` accepts
one hostname or IPv4/IPv6 address, not a multi-host list.

---

## Migration Rules

- Backward compatibility first: avoid breaking running workers/controllers mid-rollout.
- For destructive schema evolution, use staged rollout (expand -> migrate -> contract).
- Keep DDL and code changes in the same PR/changeset when tightly coupled.
- Regenerate code artifacts after schema changes (Ent/sqlc as applicable).
- Manual Atlas apply is reserved for development diagnostics or emergency
  operator-controlled repair where the database already has the base business
  schema or an Atlas revision table. Routine production upgrades must use the
  released server artifact startup path.

## Batch Retry and Idempotency Rollout {#batch-retry-and-idempotency-rollout}

The batch concurrency hardening keeps the existing persistence contract
compatible with historical data:

- `tickets.attempt_count` is additive, non-null, and defaults to `0`;
  `tickets.last_attempt_at` is additive and nullable.
- The migration backfills one logical attempt only for legacy child rows whose
  state proves that dispatch started. Rows whose history cannot prove a
  dispatch retain the conservative lower bound.
- Atlas does not alter `batch_tickets.request_id` at all. Its PostgreSQL `text`
  schema and every historical value remain unchanged, including `NULL`, empty,
  whitespace-only, duplicate, and long values; there is no normalization,
  truncation, clearing, or type conversion in this migration.
- This change does not add a digest column or a database unique index for the
  idempotency tuple. New-version writers serialize submission, recheck the
  exact `(created_by, requested operation, request_id)` tuple against the
  persisted parent payload after acquiring the transaction-scoped advisory
  locks, and persist the batch in that same transaction. Power `START`, `STOP`,
  and `RESTART` remain distinct operation scopes.

The application-only guard cannot coordinate with an older writer that still
uses the former read-before-write path. A brief mixed-version interval therefore
retains the pre-existing duplicate-submit race and must not be described as
providing the new idempotency guarantee. The attempt-only migration does not
lose or rewrite request-ID data, but operators must still drain old and new
batch mutation paths in stages before relying on serialized replay behavior.
The new parent dispatcher is isolated on the `batch_approval_dispatch` River
queue. Old binaries do not register that queue and therefore cannot reserve or
discard a new job kind they do not understand.

### Historical Replay Payload Integrity Gate

Before enabling any new-version mutation endpoint, inventory every
`batch_tickets` row with a non-empty trimmed `request_id` and prove all of the
following without rewriting historical values:

- exactly one root `tickets` row exists for the projection ID, its
  `parent_ticket_id` is null, and exactly one matching `domain_events` row has
  `aggregate_type = 'batch'` and `aggregate_id` equal to that parent ID;
- the event payload decodes to the expected batch request shape, contains a
  non-empty normalized operation, and its operation agrees with the parent
  ticket operation, event type, and projection batch type;
- the normalized payload `request_id` equals the projection value and the
  durable requester/creator fields identify the same actor; and
- grouping by exact `(created_by, normalized operation, trimmed request_id)`
  reveals no unexplained duplicate logical request. Keep deterministic
  `created_at, id` ordering in the inventory so any approved canonical-history
  decision is reproducible.

Missing parents/events, malformed or operation-less payloads, tuple mismatches,
and ambiguous duplicates are rollout blockers. Export them to the change
record, keep mutation endpoints disabled, and reconcile them through a reviewed
forward-fix. Do not silently skip a malformed matching row, delete a duplicate,
or normalize/overwrite its historical request ID merely to make the new replay
lookup pass.

The new-version runtime also fails closed when an idempotency lookup encounters
a matching projection with a missing root/event, an inconsistent durable actor
or operation identity, or a malformed/mismatched payload. It compares request
IDs with boundary whitespace trimmed (including historical rows) but never
rewrites the stored text. This defense prevents a damaged history row from
authorizing a new duplicate write; it does not replace the pre-rollout inventory
or the reviewed forward-fix required by this gate.

Committed-response recovery first performs a bounded, read-only replay lookup
after endpoint authorization and request normalization but before mutable
namespace, VM, catalog, approval-policy, or live-state preparation. A miss from
that fast path does not authorize creation: concurrent first submissions still
pass through the process-local submission gate and the transaction-scoped
cluster-wide, actor, and exact-key locks, followed by the authoritative replay
recheck in that transaction. Both lookups use the same SQL predicate with the non-unique
`(created_by, batch_type, sha256(normalized request_id))` expression index and
exact `BTRIM(request_id, shared_cutset) = normalized_request_id` equality.
Hash collisions cannot authorize replay: the exact predicate excludes them before
rows enter the candidate set or graph validation. The runtime loads at most 65
rows for the same exact normalized key. The 65th exact-key row is only an
overflow sentinel: more than 64 historical duplicates is an integrity error,
and the runtime stops before graph validation. Only 64 exact-key rows or fewer
are validated against the root Ticket, parent Event, durable actor, operation,
and payload identities before selecting the deterministic oldest match. The
digest is never stored or trusted as identity. This bounds each lookup's
historical-scan and per-request graph work while preserving opaque, unbounded
PostgreSQL `text` keys; it is not a substitute for the planned operation-aware
uniqueness mechanism.

### Legacy VM Power `dispatch_mode` Admission Gate

`VMPowerPayload.dispatch_mode` is immutable submission provenance. The new
worker accepts only `direct` or `ticket`; a missing or invalid value is not a
legacy default and causes that delivery to be cancelled without changing the
Event/Ticket fence. A new-release instance automatically registers this worker
on the shared `vm_operations` queue, so there is no safe worker-only enable or
disable step.

Enter a full VM-power maintenance window before inventory or drain. Freeze all
old-release producers: direct VM power submissions, ordinary and external
approval decisions that can enqueue power work, batch power submission and
retry, and every other VM-power create/approve/retry path. Also freeze the
general batch submit, retry, and cancel mutations. Read-only status may remain
available. After this freeze, inventory every non-terminal VM power Event,
including one with no current River job, and every runnable `vm_power` job
(`available`, `pending`, `retryable`, `running`, or `scheduled`). Join each
job's `EventID` to its Event, Ticket (when present), parent/projection, and
immutable payload, then classify the durable dispatch mode.

While only old-release instances remain running, their VM power workers must
exclusively and fully consume and drain every runnable `vm_power` job whose
legacy payload has a missing or invalid mode. A new-release worker must not
consume such a job. Handle legacy work that cannot drain as follows:

- For a ticket-backed `PENDING` Event with a missing or invalid mode that was not
  already drained before the producer freeze, use the reviewed normal
  application cancellation/termination flow, then preserve the original
  evidence. Submit a fresh request only after the new version is running.
- A direct `PENDING` Event with no Ticket and no runnable old-worker job has no
  supported application transition in the current release. Its discovery is a
  hard rollout blocker, not a quarantine candidate: keep the new release
  stopped and do not claim the admission gate passed. A separately implemented
  and reviewed reconciliation capability would be required before re-running
  the inventory; this runbook does not assume that such a utility exists.
- For a `PROCESSING` START/STOP Event, let the originating old worker converge
  the durable result. If its completion or provider effect cannot be proved,
  quarantine the job and workflow while preserving the Event, Ticket, payload,
  worker, and provider evidence for reviewed reconciliation.
- For a `PROCESSING` RESTART Event, permanently preserve the
  `PROCESSING/EXECUTING` fence and use only read-only provider verification.
  Quarantine the workflow; never fill the mode, rewrite it terminal, release the
  fence, or replay/redispatch the operation.

Never mutate immutable `DomainEvent.payload`, use direct SQL, perform a blind
backfill, or reinsert a River job to retrofit provenance. No unresolved
`PENDING` legacy item with missing/invalid mode may be carried across the
release boundary.

Quarantine is reserved for unsafe `PROCESSING` work. It means an
operator-reviewed queue action that makes the affected job non-runnable while
retaining its Event/Ticket and immutable payload evidence; it does not authorize
manual workflow-row edits or blind job insertion. Before any new-release
instance starts, the admission report must prove both zero unresolved `PENDING`
Events (with or without Tickets) with a missing or invalid `dispatch_mode` and
zero runnable `vm_power` jobs that reference one. Every retained legacy
`PROCESSING` item must be named in the quarantine record with the evidence and
handling above.

### Rolling Upgrade Order

1. Take a database backup and record the initial inventory of non-terminal batch
   children, non-terminal VM power Events, runnable VM power jobs,
   `PROCESSING` restart events, and every existing `batch_approval_dispatch`
   River job.
2. Enter the full maintenance window described above. Freeze every old-version
   path that can submit, approve, or retry VM power work, including direct,
   external-approval, and batch paths; also freeze batch cancel/mutation traffic.
   Read-only status may remain available.
3. While no producer can create more legacy work and only old-release instances
   are running, complete the historical replay-payload gate and the
   `dispatch_mode` inventory. Let old workers exclusively drain legacy runnable
   jobs, normally terminate every supported `PENDING` legacy request with
   missing/invalid mode, treat any unsupported direct orphan as a hard rollout
   blocker, and quarantine only unsafe `PROCESSING` work as specified above.
   Prove both admission counts are zero, then completely stop all old-release
   instances.
4. Only after step 3 succeeds, start the new release through the normal startup
   migration path. Its `VMPowerWorker` registers automatically and may now
   consume only provenance-complete jobs; verify that the release also consumes
   the dedicated `batch_approval_dispatch` queue.
5. Verify the migrated column types/defaults, attempt backfill, absence of old
   instances, historical payload report, zero-unresolved-PENDING and
   zero-invalid-runnable VM power admission report, ambiguous restart inventory,
   and the dispatcher recovery checks below. Then re-enable the VM power and
   batch mutation paths.
6. During the rollout, reuse the same `request_id` for transport retries. A
   response lost after commit must be recovered by the post-lock exact replay
   lookup, not by submitting a new key.

### Rollback and Forward-Fix

- Before new workers or mutation endpoints are enabled, binary rollback is
  safe: keep the additive columns and return traffic to the drained release.
- After the new release has produced attempt counters or claimed a restart
  `PROCESSING` fence, do not start old power workers immediately. Freeze batch
  mutations, drain the new workers, preserve the non-terminal restart evidence,
  and inspect provider state independently. An old worker does not understand
  the fail-closed fence and could repeat an ambiguous provider action. There is
  no release endpoint; do not release the fence with direct SQL.
- Keep the additive migration in place during application rollback. Dropping
  attempt history is destructive and is not required for old binaries, which
  ignore the additional columns.
- Prefer a forward fix when ambiguous restart fences or new logical attempts
  exist. Resume mutation traffic only after ticket, event, parent aggregate,
  and River job state agree.
- Database-enforced idempotency is a separate, two-phase follow-up after all old
  writers have permanently exited: first inventory historical duplicates and
  approve their non-destructive handling, then introduce and validate a
  compatible operation-aware uniqueness mechanism while preserving
  `request_id` as `text`.
  This release must not pre-accept the future unique-index representation,
  mutate historical keys, or narrow the public key contract. The non-unique
  SHA-256 expression index is only an exact-predicate-guarded lookup
  accelerator; hash collisions are filtered out and never authorize replay.

### Dispatcher Recovery and Reconciliation

The dedicated dispatcher deliberately cancels a consistency-unsafe job without
running the generic failure finalizer. River may therefore contain a
`cancelled` or `discarded` `batch_approval_dispatch` job while durable child
rows remain `PENDING`. Treat queue state and workflow state as a joined recovery
problem:

1. Freeze batch approval, retry, and cancel mutations and snapshot the affected
   parent, parent Event, `batch_tickets` projection, all child Ticket/Event
   pairs, and every dispatcher job whose args carry that parent ID.
2. Classify dispatcher jobs into runnable (`available`, `pending`, `retryable`,
   `running`, `scheduled`) and terminal (`completed`, `cancelled`, `discarded`)
   states. Confirm at most one runnable parent-keyed job exists on the dedicated
   queue.
3. Recompute child counts and compare all three parent views. A terminal job
   with no `PENDING`/active child and matching terminal parents needs no new
   work. A cancelled/discarded job with `PENDING` children, or any terminal
   parent with active children, is inconsistent and remains quarantined.
4. For non-restart dispatcher inconsistencies, do not blindly insert another
   dispatcher or directly edit River rows. First
   prove the parent identity, normalized execution snapshot, original approver,
   child operation/Event pairing, attempt budget, and raw parent
   `EXECUTING|PROCESSING` eligibility. If all evidence is unambiguous, apply a
   reviewed application-level forward-fix that uses the same parent lock,
   expected-state transitions, projection refresh, and transactional River
   insertion as the normal path. Otherwise close the old request through an
   approved terminal reconciliation and require a new batch/approval request.
5. For `PROCESSING` restart events, never infer provider non-application from a
   cancelled/discarded job and never use the generic forward-fix above. Drain
   the originating worker and inspect provider state independently, but keep
   the durable fence: no API can release the fence, and manual workflow-row
   edits or redispatch are prohibited. A safe release remains deferred until a
   provider receipt/idempotency or provable-cancellation protocol can exclude a
   late provider effect. Until then, quarantine and never redispatch the VM
   power operation.
6. Resume traffic only when the change record proves parent Ticket, parent
   Event, projection counters/status, every child pair, attempt counts, and
   River jobs agree. Retain before/after evidence and the reviewer identity.

---

## Rollout Checklist

- Migration scripts reviewed and reproducible in local/dev environments.
- No prohibited manual DDL in app startup path.
- CI checks pass for schema/codegen/governance scripts.
- Rollback/mitigation notes included for non-trivial changes.
- Mixed-version writers/workers are explicitly drained when a migration's
  concurrency semantics cannot be honored by the old binary.
- All old-version VM power producer paths are frozen before drain and remain
  frozen until every unresolved `PENDING` legacy item is normally terminated
  and every runnable `vm_power` job references an immutable `dispatch_mode` of
  `direct` or `ticket`. Only then are old instances stopped and the new release
  started; unsafe `PROCESSING` exceptions remain explicitly quarantined without
  payload backfill or redispatch.
- Any direct `PENDING` Event with no Ticket and no runnable old-worker job is a
  hard rollout blocker because the current release has no supported
  reconciliation path for it; it is never silently waived or quarantined.
- Every non-empty historical batch request ID has a valid operation-bearing
  parent payload and any duplicate tuple has a reviewed disposition.
- Cancelled/discarded dedicated dispatchers are joined to parent/child state;
  no pending child is stranded behind a terminal job without a recovery record.

---

## Operational Notes

- Monitor queue table bloat/autovacuum (`river_*`) and audit table growth.
- Ensure retention/archival jobs are aligned with lifecycle policy.

See:

- [00-prerequisites.md §6 Database Connection](../phases/00-prerequisites.md#6-database-connection)
- [00-prerequisites.md §Migration Verification](../phases/00-prerequisites.md#migration-verification-developmentci)
- [04-governance.md §1 Database Migration](../phases/04-governance.md#1-database-migration)
- [04-governance.md §2 River Queue](../phases/04-governance.md#2-river-queue-adr-0006)
