package usecase

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"kv-shepherd.io/shepherd/internal/domain"
)

func TestApprovalAtomicWriterFailBatchApprovalChildDispatch_TerminalizesExactChild(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_dispatch_failure_terminal")
	parentID, childID, eventID, guard := seedBatchApprovalPendingDispatch(t, store, "terminal")

	err := store.writer.FailBatchApprovalChildDispatch(
		t.Context(),
		guard,
		childID,
		eventID,
		" "+guard.Approver+" ",
		domain.BatchApprovalDispatchFailureUnsupported,
	)
	if err != nil {
		t.Fatalf("FailBatchApprovalChildDispatch() error = %v", err)
	}

	assertBatchApprovalChildState(t, store.pool, childID, "FAILED", "FAILED", 1)
	assertBatchApprovalChildRetryMetadata(
		t,
		store.pool,
		childID,
		domain.BatchApprovalDispatchFailureUnsupported,
		true,
	)
	var approver string
	if err := store.pool.QueryRow(
		t.Context(),
		`SELECT COALESCE(approver, '') FROM tickets WHERE id = $1`,
		childID,
	).Scan(&approver); err != nil {
		t.Fatalf("load failed child approver: %v", err)
	}
	if approver != guard.Approver {
		t.Fatalf("failed child approver = %q, want %q", approver, guard.Approver)
	}
	if state := loadBatchApprovalDispatchState(t, store.pool, parentID); state.parentStatus != "EXECUTING" || state.parentEvent != "PROCESSING" {
		t.Fatalf("failure terminalization changed parent execution state: %+v", state)
	}
}

func TestApprovalAtomicWriterFailBatchApprovalChildDispatch_RejectsUnsafeReasonWithoutWrites(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_dispatch_failure_reason")
	parentID, childID, eventID, guard := seedBatchApprovalPendingDispatch(t, store, "reason")
	before := loadBatchApprovalGraphSnapshot(t, store.pool, parentID)

	err := store.writer.FailBatchApprovalChildDispatch(
		t.Context(),
		guard,
		childID,
		eventID,
		guard.Approver,
		"provider returned private infrastructure detail",
	)
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("unsafe public reason error = %v, want allowlist rejection", err)
	}
	if after := loadBatchApprovalGraphSnapshot(t, store.pool, parentID); after != before {
		t.Fatalf("unsafe public reason changed durable graph\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestApprovalAtomicWriterFailBatchApprovalChildDispatch_RejectsStaleGraphGuard(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_dispatch_failure_stale_guard")
	parentID, childID, eventID, guard := seedBatchApprovalPendingDispatch(t, store, "stale")
	if _, err := store.pool.Exec(
		t.Context(),
		`UPDATE tickets SET selected_cluster_id = 'cluster-after-validation' WHERE id = $1`,
		parentID,
	); err != nil {
		t.Fatalf("mutate parent after graph validation: %v", err)
	}
	before := loadBatchApprovalGraphSnapshot(t, store.pool, parentID)

	err := store.writer.FailBatchApprovalChildDispatch(
		t.Context(),
		guard,
		childID,
		eventID,
		guard.Approver,
		domain.BatchApprovalDispatchFailureValidation,
	)
	if err == nil || !strings.Contains(err.Error(), "graph changed after validation") {
		t.Fatalf("stale graph guard error = %v, want exact graph rejection", err)
	}
	if after := loadBatchApprovalGraphSnapshot(t, store.pool, parentID); after != before {
		t.Fatalf("stale graph rejection changed durable graph\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestApprovalAtomicWriterFailBatchApprovalChildDispatch_RollsBackTicketWhenEventCASLoses(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_dispatch_failure_event_cas")
	parentID, childID, eventID, guard := seedBatchApprovalPendingDispatch(t, store, "event-cas")
	installBatchApprovalSafetyTrigger(t, store, `
CREATE FUNCTION force_child_event_cas_loss() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  UPDATE domain_events SET status = 'FAILED' WHERE id = OLD.event_id;
  RETURN NEW;
END;
$$;
CREATE TRIGGER force_child_event_cas_loss
BEFORE UPDATE OF status ON tickets
FOR EACH ROW
WHEN (OLD.status = 'PENDING' AND NEW.status = 'FAILED')
EXECUTE FUNCTION force_child_event_cas_loss();
`)
	before := loadBatchApprovalGraphSnapshot(t, store.pool, parentID)

	err := store.writer.FailBatchApprovalChildDispatch(
		t.Context(),
		guard,
		childID,
		eventID,
		guard.Approver,
		domain.BatchApprovalDispatchFailureExhausted,
	)
	if err == nil || !strings.Contains(err.Error(), "child event") || !strings.Contains(err.Error(), "got 0") {
		t.Fatalf("event CAS loss error = %v, want zero-row event transition", err)
	}
	if after := loadBatchApprovalGraphSnapshot(t, store.pool, parentID); after != before {
		t.Fatalf("event CAS loss did not roll back ticket and event\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestApprovalAtomicWriterFailBatchApprovalChildDispatch_RollsBackDeferredCommitFailure(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_dispatch_failure_commit")
	parentID, childID, eventID, guard := seedBatchApprovalPendingDispatch(t, store, "commit")
	installBatchApprovalSafetyTrigger(t, store, `
CREATE FUNCTION reject_child_failure_commit() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'forced child failure commit rejection';
END;
$$;
CREATE CONSTRAINT TRIGGER reject_child_failure_commit
AFTER UPDATE ON domain_events
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION reject_child_failure_commit();
`)
	before := loadBatchApprovalGraphSnapshot(t, store.pool, parentID)

	err := store.writer.FailBatchApprovalChildDispatch(
		t.Context(),
		guard,
		childID,
		eventID,
		guard.Approver,
		domain.BatchApprovalDispatchFailureValidation,
	)
	if err == nil || !strings.Contains(err.Error(), "commit batch approval child failure") {
		t.Fatalf("deferred commit error = %v, want commit failure", err)
	}
	if after := loadBatchApprovalGraphSnapshot(t, store.pool, parentID); after != before {
		t.Fatalf("deferred commit failure did not roll back graph\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_CASConflictsRollBackGraphAndJob(t *testing.T) {
	tests := []struct {
		name          string
		trigger       string
		wantExhausted bool
	}{
		{
			name: "selected child disappears after exact validation",
			trigger: `
CREATE FUNCTION remove_retry_child_before_cas() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM tickets WHERE id = OLD.id;
  RETURN NULL;
END;
$$;
CREATE TRIGGER remove_retry_child_before_cas
BEFORE UPDATE OF status ON tickets
FOR EACH ROW
WHEN (OLD.status = 'FAILED' AND NEW.status = 'PENDING')
EXECUTE FUNCTION remove_retry_child_before_cas();
`,
		},
		{
			name: "selected child state changes after exact validation",
			trigger: `
CREATE FUNCTION change_retry_child_before_cas() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF pg_trigger_depth() > 1 THEN
    RETURN NEW;
  END IF;
  UPDATE tickets SET status = 'SUCCESS' WHERE id = OLD.id;
  RETURN NULL;
END;
$$;
CREATE TRIGGER change_retry_child_before_cas
BEFORE UPDATE OF status ON tickets
FOR EACH ROW
WHEN (OLD.status = 'FAILED' AND NEW.status = 'PENDING')
EXECUTE FUNCTION change_retry_child_before_cas();
`,
		},
		{
			name: "selected child exhausts attempts after exact validation",
			trigger: `
CREATE FUNCTION exhaust_retry_child_before_cas() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  UPDATE tickets SET attempt_count = 3 WHERE id = OLD.id;
  RETURN NULL;
END;
$$;
CREATE TRIGGER exhaust_retry_child_before_cas
BEFORE UPDATE OF status ON tickets
FOR EACH ROW
WHEN (OLD.status = 'FAILED' AND NEW.status = 'PENDING')
EXECUTE FUNCTION exhaust_retry_child_before_cas();
`,
			wantExhausted: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newApprovalAtomicBehaviorStore(t, "batch_retry_cas_conflict")
			parentID, childID, eventID := seedBatchApprovalFailedRetry(t, store, "cas-conflict")
			installBatchApprovalSafetyTrigger(t, store, tc.trigger)
			before := loadBatchApprovalGraphSnapshot(t, store.pool, parentID)

			err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
				ParentTicketID: parentID,
				ParentEventID:  parentID + "-event",
				Approver:       "retry-approver",
				Children: []domain.BatchApprovalRetryChild{{
					TicketID: childID,
					EventID:  eventID,
				}},
			})
			if tc.wantExhausted {
				var exhausted *BatchChildAttemptsExhaustedError
				if !errors.As(err, &exhausted) || exhausted.AttemptCount != domain.BatchChildMaxAttempts {
					t.Fatalf("retry CAS error = %v, want attempts-exhausted conflict", err)
				}
			} else {
				var notEligible *BatchApprovalRetryNotEligibleError
				if !errors.As(err, &notEligible) {
					t.Fatalf("retry CAS error = %v, want not-eligible conflict", err)
				}
			}
			if after := loadBatchApprovalGraphSnapshot(t, store.pool, parentID); after != before {
				t.Fatalf("retry CAS conflict did not roll back graph\nbefore: %s\nafter:  %s", before, after)
			}
			assertBatchApprovalDispatcherJobs(t, store.riverClient, parentID, 0)
		})
	}
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_ValidatesBeforePersistence(t *testing.T) {
	valid := domain.BatchApprovalRetryInput{
		ParentTicketID: "parent",
		ParentEventID:  "parent-event",
		Approver:       "approver",
		Children: []domain.BatchApprovalRetryChild{{
			TicketID: "child",
			EventID:  "child-event",
		}},
	}
	if err := (&ApprovalAtomicWriter{}).RetryBatchApprovalAndEnqueue(t.Context(), valid); err == nil ||
		!strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("uninitialized retry error = %v, want initialization failure", err)
	}

	incomplete := valid
	incomplete.Children = nil
	if err := (&ApprovalAtomicWriter{}).RetryBatchApprovalAndEnqueue(t.Context(), incomplete); err == nil ||
		!strings.Contains(err.Error(), "input is incomplete") {
		t.Fatalf("incomplete retry error = %v, want validation failure", err)
	}

	conflicting := valid
	conflicting.Children = append(conflicting.Children, domain.BatchApprovalRetryChild{
		TicketID: "child",
		EventID:  "other-event",
	})
	store := newApprovalAtomicBehaviorStore(t, "batch_retry_identity_validation")
	if err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), conflicting); err == nil ||
		!strings.Contains(err.Error(), "multiple requested events") {
		t.Fatalf("conflicting retry identity error = %v, want exact binding rejection", err)
	}
}

func TestBatchApprovalChildGraphIdentityMatches_MapsExactPayloads(t *testing.T) {
	t.Parallel()

	createPayload := domain.VMCreationPayload{
		RequesterID:    "actor",
		ServiceID:      " target-create ",
		TemplateID:     "template-a",
		InstanceSizeID: "size-a",
		Namespace:      "namespace-a",
		TargetCPUCores: 2,
		TargetMemoryGi: 0,
		TargetDiskGB:   20,
	}
	createIdentity, ok := batchApprovalChildGraphIdentityMatches(
		lockedBatchApprovalChild{Operation: batchOperationCreate, Requester: " actor "},
		lockedBatchApprovalEvent{
			EventType:     string(domain.EventVMCreationRequested),
			AggregateType: "vm",
			AggregateID:   "target-create",
			Payload:       marshalBatchApprovalSafetyPayload(t, createPayload),
		},
	)
	if !ok || createIdentity.Target != "target-create" || createIdentity.PowerAction != "" {
		t.Fatalf("create identity = %+v, ok=%t, want exact non-power target", createIdentity, ok)
	}
	var createItem domain.BatchVMItemPayload
	if err := json.Unmarshal([]byte(createIdentity.ItemKey), &createItem); err != nil {
		t.Fatalf("decode canonical create item: %v", err)
	}
	if createItem.ServiceID != "target-create" || createItem.OwnerID != "actor" ||
		createItem.TargetCPUCores == nil || *createItem.TargetCPUCores != 2 ||
		createItem.TargetMemoryGi != nil || createItem.TargetDiskGB == nil || *createItem.TargetDiskGB != 20 {
		t.Fatalf("canonical create item = %+v, want normalized exact execution inputs", createItem)
	}

	deletePayload := domain.VMDeletePayload{VMID: "target-delete", Actor: "actor", Namespace: "namespace-b"}
	deleteIdentity, ok := batchApprovalChildGraphIdentityMatches(
		lockedBatchApprovalChild{Operation: batchOperationDelete, Requester: "actor"},
		lockedBatchApprovalEvent{
			EventType:     string(domain.EventVMDeletionRequested),
			AggregateType: "vm",
			AggregateID:   "target-delete",
			Payload:       marshalBatchApprovalSafetyPayload(t, deletePayload),
		},
	)
	if !ok || deleteIdentity.Target != "target-delete" || !strings.Contains(deleteIdentity.ItemKey, `"vm_id":"target-delete"`) {
		t.Fatalf("delete identity = %+v, ok=%t, want exact delete target", deleteIdentity, ok)
	}

	for _, tc := range []struct {
		name      string
		action    string
		eventType string
	}{
		{name: "stop", action: "stop", eventType: string(domain.EventVMStopRequested)},
		{name: "restart", action: "restart", eventType: string(domain.EventVMRestartRequested)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity, matched := batchApprovalChildGraphIdentityMatches(
				lockedBatchApprovalChild{Operation: "POWER", Requester: "actor"},
				lockedBatchApprovalEvent{
					EventType:     tc.eventType,
					AggregateType: "vm",
					AggregateID:   "target-power",
					Payload: marshalBatchApprovalSafetyPayload(t, domain.VMPowerPayload{
						VMID: "target-power", Actor: "actor", Operation: " " + tc.action + " ",
					}),
				},
			)
			if !matched || identity.Target != "target-power" || identity.PowerAction != tc.action ||
				!strings.Contains(identity.ItemKey, `"operation":"`+tc.action+`"`) {
				t.Fatalf("power identity = %+v, matched=%t, want action %q", identity, matched, tc.action)
			}
		})
	}
}

func TestBatchApprovalChildGraphIdentityMatches_RejectsIdentityDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		child lockedBatchApprovalChild
		event lockedBatchApprovalEvent
	}{
		{
			name:  "create event type",
			child: lockedBatchApprovalChild{Operation: batchOperationCreate, Requester: "actor"},
			event: lockedBatchApprovalEvent{
				EventType: string(domain.EventVMModifyRequested), AggregateType: "vm", AggregateID: "target",
				Payload: marshalBatchApprovalSafetyPayload(t, domain.VMCreationPayload{ServiceID: "target", RequesterID: "actor"}),
			},
		},
		{
			name:  "create unknown payload field",
			child: lockedBatchApprovalChild{Operation: batchOperationCreate, Requester: "actor"},
			event: lockedBatchApprovalEvent{
				EventType: string(domain.EventVMCreationRequested), AggregateType: "vm", AggregateID: "target",
				Payload: []byte(`{"service_id":"target","requester_id":"actor","unknown":true}`),
			},
		},
		{
			name:  "delete target",
			child: lockedBatchApprovalChild{Operation: batchOperationDelete, Requester: "actor"},
			event: lockedBatchApprovalEvent{
				EventType: string(domain.EventVMDeletionRequested), AggregateType: "vm", AggregateID: "target",
				Payload: marshalBatchApprovalSafetyPayload(t, domain.VMDeletePayload{VMID: "other", Actor: "actor"}),
			},
		},
		{
			name:  "delete event type",
			child: lockedBatchApprovalChild{Operation: batchOperationDelete, Requester: "actor"},
			event: lockedBatchApprovalEvent{
				EventType: string(domain.EventVMModifyRequested), AggregateType: "vm", AggregateID: "target",
				Payload: marshalBatchApprovalSafetyPayload(t, domain.VMDeletePayload{VMID: "target", Actor: "actor"}),
			},
		},
		{
			name:  "power operation",
			child: lockedBatchApprovalChild{Operation: "POWER", Requester: "actor"},
			event: lockedBatchApprovalEvent{
				EventType: string(domain.EventVMStartRequested), AggregateType: "vm", AggregateID: "target",
				Payload: marshalBatchApprovalSafetyPayload(t, domain.VMPowerPayload{VMID: "target", Actor: "actor", Operation: "suspend"}),
			},
		},
		{
			name:  "power event type",
			child: lockedBatchApprovalChild{Operation: "POWER", Requester: "actor"},
			event: lockedBatchApprovalEvent{
				EventType: string(domain.EventVMStartRequested), AggregateType: "vm", AggregateID: "target",
				Payload: marshalBatchApprovalSafetyPayload(t, domain.VMPowerPayload{VMID: "target", Actor: "actor", Operation: "restart"}),
			},
		},
		{
			name:  "power actor",
			child: lockedBatchApprovalChild{Operation: "POWER", Requester: "actor"},
			event: lockedBatchApprovalEvent{
				EventType: string(domain.EventVMStartRequested), AggregateType: "vm", AggregateID: "target",
				Payload: marshalBatchApprovalSafetyPayload(t, domain.VMPowerPayload{VMID: "target", Actor: "other", Operation: "start"}),
			},
		},
		{
			name:  "unsupported child operation",
			child: lockedBatchApprovalChild{Operation: "SNAPSHOT", Requester: "actor"},
			event: lockedBatchApprovalEvent{
				EventType: "VM_SNAPSHOT_REQUESTED", AggregateType: "vm", AggregateID: "target", Payload: []byte(`{}`),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if identity, ok := batchApprovalChildGraphIdentityMatches(tc.child, tc.event); ok {
				t.Fatalf("identity drift matched as %+v", identity)
			}
		})
	}
}

func TestDecodeBatchApprovalPayloadExact_RejectsNonCanonicalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		wantErr string
	}{
		{name: "unknown field", payload: `{"name":"one","extra":true}`, wantErr: "unknown field"},
		{name: "multiple values", payload: `{"name":"one"} {"name":"two"}`, wantErr: "multiple JSON values"},
		{name: "invalid trailing value", payload: `{"name":"one"} {`, wantErr: "unexpected EOF"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var destination struct {
				Name string `json:"name"`
			}
			err := decodeBatchApprovalPayloadExact([]byte(tc.payload), &destination)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("decode exact payload error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestBatchApprovalExecutionFromLockedParent_EnvelopeBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		modified    string
		wantErr     bool
		wantCluster string
		wantStorage string
		wantModes   string
	}{
		{name: "absent envelope", wantCluster: "cluster-column", wantStorage: "storage-column"},
		{name: "null envelope", modified: `null`, wantCluster: "cluster-column", wantStorage: "storage-column"},
		{name: "missing execution key", modified: `{"audit":"preserved"}`, wantCluster: "cluster-column", wantStorage: "storage-column"},
		{name: "null execution", modified: `{"batch_approval_execution":null}`, wantCluster: "cluster-column", wantStorage: "storage-column"},
		{name: "malformed outer envelope", modified: `{`, wantErr: true},
		{name: "malformed execution", modified: `{"batch_approval_execution":"invalid"}`, wantErr: true},
		{
			name: "valid execution overrides columns",
			modified: `{"batch_approval_execution":{"cluster_id":" cluster-envelope ","storage_class":" storage-envelope ",` +
				`"dv_access_modes":[" ReadWriteMany ","","ReadWriteOnce"]}}`,
			wantCluster: "cluster-envelope",
			wantStorage: "storage-envelope",
			wantModes:   "ReadWriteMany,ReadWriteOnce",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			execution, err := batchApprovalExecutionFromLockedParent(batchApprovalParentIdentity{
				SelectedCluster: pgtype.Text{String: " cluster-column ", Valid: true},
				SelectedStorage: pgtype.Text{String: " storage-column ", Valid: true},
				ModifiedSpec:    []byte(tc.modified),
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("batchApprovalExecutionFromLockedParent() error = nil, want malformed envelope error")
				}
				return
			}
			if err != nil {
				t.Fatalf("batchApprovalExecutionFromLockedParent() error = %v", err)
			}
			if execution.ClusterID != tc.wantCluster || execution.StorageClass != tc.wantStorage ||
				strings.Join(execution.DVAccessModes, ",") != tc.wantModes {
				t.Fatalf("execution = %+v, want cluster=%q storage=%q modes=%q", execution, tc.wantCluster, tc.wantStorage, tc.wantModes)
			}
		})
	}
}

func TestNormalizeBatchSelectedChildren_ExactIdentitySet(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeBatchSelectedChildren([]batchApprovalSelectedChild{
		{TicketID: " child-b ", EventID: " event-b "},
		{TicketID: "child-a", EventID: "event-a"},
		{TicketID: "child-b", EventID: "event-b"},
	}, "selected batch")
	if err != nil {
		t.Fatalf("normalize exact child set: %v", err)
	}
	if len(normalized) != 2 || normalized[0].TicketID != "child-a" || normalized[0].EventID != "event-a" ||
		normalized[1].TicketID != "child-b" || normalized[1].EventID != "event-b" {
		t.Fatalf("normalized child set = %+v, want trimmed, de-duplicated ticket order", normalized)
	}

	for _, tc := range []struct {
		name     string
		children []batchApprovalSelectedChild
		wantErr  string
	}{
		{name: "incomplete", children: []batchApprovalSelectedChild{{TicketID: "child"}}, wantErr: "child 0 is incomplete"},
		{
			name: "ticket rebound",
			children: []batchApprovalSelectedChild{
				{TicketID: "child", EventID: "event-a"},
				{TicketID: "child", EventID: "event-b"},
			},
			wantErr: "multiple requested events",
		},
		{
			name: "event rebound",
			children: []batchApprovalSelectedChild{
				{TicketID: "child-a", EventID: "event"},
				{TicketID: "child-b", EventID: "event"},
			},
			wantErr: "requested by tickets",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, normalizeErr := normalizeBatchSelectedChildren(tc.children, "selected batch")
			if normalizeErr == nil || !strings.Contains(normalizeErr.Error(), tc.wantErr) {
				t.Fatalf("normalize error = %v, want %q", normalizeErr, tc.wantErr)
			}
		})
	}
}

func TestBatchApprovalDurableStatePairHelpers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		ticket     string
		event      string
		projection string
		want       bool
	}{
		{name: "executing", ticket: "EXECUTING", event: "PROCESSING", projection: "IN_PROGRESS", want: true},
		{name: "success", ticket: "SUCCESS", event: "COMPLETED", projection: "COMPLETED", want: true},
		{name: "failed", ticket: "FAILED", event: "FAILED", projection: "FAILED", want: true},
		{name: "partial failure", ticket: "FAILED", event: "FAILED", projection: "PARTIAL_SUCCESS", want: true},
		{name: "cancelled", ticket: "CANCELLED", event: "CANCELLED", projection: "CANCELLED", want: true},
		{name: "crossed terminal pair", ticket: "SUCCESS", event: "FAILED", projection: "COMPLETED"},
		{name: "unsupported", ticket: "PENDING", event: "PENDING", projection: "PENDING_APPROVAL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := batchApprovalDispatchParentStateAllowed(tc.ticket, tc.event, tc.projection); got != tc.want {
				t.Fatalf("parent state allowed = %t, want %t for (%s,%s,%s)", got, tc.want, tc.ticket, tc.event, tc.projection)
			}
		})
	}

	for _, tc := range []struct {
		ticket string
		event  string
		want   bool
	}{
		{ticket: "PENDING", event: "PENDING", want: true},
		{ticket: "APPROVED", event: "PROCESSING", want: true},
		{ticket: "EXECUTING", event: "PENDING", want: true},
		{ticket: "SUCCESS", event: "COMPLETED", want: true},
		{ticket: "FAILED", event: "CANCELLED", want: true},
		{ticket: "REJECTED", event: "CANCELLED", want: true},
		{ticket: "CANCELLED", event: "CANCELLED", want: true},
		{ticket: "FAILED", event: "COMPLETED"},
		{ticket: "UNKNOWN", event: "PENDING"},
	} {
		if got := batchApprovalDispatchChildStateAllowed(tc.ticket, tc.event); got != tc.want {
			t.Fatalf("child state allowed = %t, want %t for (%s,%s)", got, tc.want, tc.ticket, tc.event)
		}
	}
}

func TestBatchApprovalRetryStableSiblingStateHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mode   batchApprovalIdentityMode
		ticket string
		event  string
		want   bool
	}{
		{name: "success", mode: batchApprovalIdentityGenericRetry, ticket: "SUCCESS", event: "COMPLETED", want: true},
		{name: "failed", mode: batchApprovalIdentityGenericRetry, ticket: "FAILED", event: "FAILED", want: true},
		{name: "rejected", mode: batchApprovalIdentityGenericRetry, ticket: "REJECTED", event: "CANCELLED", want: true},
		{name: "generic approved processing", mode: batchApprovalIdentityGenericRetry, ticket: "APPROVED", event: "PROCESSING", want: true},
		{name: "generic approved pending", mode: batchApprovalIdentityGenericRetry, ticket: "APPROVED", event: "PENDING"},
		{name: "power approved pending", mode: batchApprovalIdentityPowerRetry, ticket: "APPROVED", event: "PENDING", want: true},
		{name: "executing pending", mode: batchApprovalIdentityGenericRetry, ticket: "EXECUTING", event: "PENDING", want: true},
		{name: "pending sibling", mode: batchApprovalIdentityGenericRetry, ticket: "PENDING", event: "PENDING"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := batchApprovalUnselectedRetryStateAllowed(tc.mode, tc.ticket, tc.event); got != tc.want {
				t.Fatalf("stable sibling state = %t, want %t for (%s,%s)", got, tc.want, tc.ticket, tc.event)
			}
		})
	}

	for _, tc := range []struct {
		parent     string
		projection string
		want       bool
	}{
		{parent: "EXECUTING", projection: "IN_PROGRESS", want: true},
		{parent: "FAILED", projection: "FAILED", want: true},
		{parent: "FAILED", projection: "PARTIAL_SUCCESS", want: true},
		{parent: "FAILED", projection: "IN_PROGRESS"},
		{parent: "SUCCESS", projection: "COMPLETED"},
	} {
		if got := batchApprovalRetryProjectionStatusMatches(tc.parent, tc.projection); got != tc.want {
			t.Fatalf("retry projection match = %t, want %t for (%s,%s)", got, tc.want, tc.parent, tc.projection)
		}
	}
}

func TestExpectedBatchApprovalParentIdentity(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		operation      string
		wantEvent      string
		wantProjection string
		wantOK         bool
	}{
		{operation: batchOperationCreate, wantEvent: string(domain.EventBatchCreateRequested), wantProjection: batchProjectionTypeCreate, wantOK: true},
		{operation: batchOperationModify, wantEvent: string(domain.EventBatchModifyRequested), wantProjection: batchProjectionTypeModify, wantOK: true},
		{operation: batchOperationDelete, wantEvent: string(domain.EventBatchDeleteRequested), wantProjection: batchProjectionTypeDelete, wantOK: true},
		{operation: "POWER", wantEvent: string(domain.EventBatchPowerRequested), wantProjection: "BATCH_POWER", wantOK: true},
		{operation: "SNAPSHOT"},
	} {
		eventType, projectionType, ok := expectedBatchApprovalParentIdentity(" " + tc.operation + " ")
		if eventType != tc.wantEvent || projectionType != tc.wantProjection || ok != tc.wantOK {
			t.Fatalf("identity(%q) = (%q,%q,%t), want (%q,%q,%t)", tc.operation, eventType, projectionType, ok, tc.wantEvent, tc.wantProjection, tc.wantOK)
		}
	}
}

func TestBatchApprovalSmallNormalizationHelpers(t *testing.T) {
	t.Parallel()

	if got := firstNonBlank(" ", " actor ", "ignored"); got != "actor" {
		t.Fatalf("firstNonBlank() = %q, want actor", got)
	}
	if got := firstNonBlank("", " "); got != "" {
		t.Fatalf("firstNonBlank(all blank) = %q, want empty", got)
	}
	if got := positiveBatchFloat(2.5); got == nil || *got != 2.5 {
		t.Fatalf("positiveBatchFloat(2.5) = %v, want pointer to 2.5", got)
	}
	if got := positiveBatchFloat(0); got != nil {
		t.Fatalf("positiveBatchFloat(0) = %v, want nil", got)
	}
	if got := positiveBatchInt(20); got == nil || *got != 20 {
		t.Fatalf("positiveBatchInt(20) = %v, want pointer to 20", got)
	}
	if got := positiveBatchInt(-1); got != nil {
		t.Fatalf("positiveBatchInt(-1) = %v, want nil", got)
	}
}

func TestApprovalAtomicWriterFailBatchApprovalChildDispatch_RequiresInitializedWriter(t *testing.T) {
	t.Parallel()

	var writer *ApprovalAtomicWriter
	err := writer.FailBatchApprovalChildDispatch(
		t.Context(),
		domain.BatchApprovalDispatchGuard{},
		"child",
		"event",
		"approver",
		domain.BatchApprovalDispatchFailureValidation,
	)
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("nil writer error = %v, want initialization failure", err)
	}
}

func seedBatchApprovalPendingDispatch(
	t *testing.T,
	store approvalAtomicBehaviorStore,
	suffix string,
) (parentID, childID, eventID string, guard domain.BatchApprovalDispatchGuard) {
	t.Helper()
	parentID = "batch-failure-" + suffix + "-parent"
	childID = "batch-failure-" + suffix + "-child"
	eventID = childID + "-event"
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     parentID,
		parentStatus: "EXECUTING",
		parentEvent:  "PROCESSING",
		batchStatus:  "IN_PROGRESS",
		children: []batchApprovalDispatchChild{{
			ticketID:     childID,
			eventID:      eventID,
			ticketState:  "PENDING",
			eventState:   "PENDING",
			attemptCount: 0,
		}},
	})
	var err error
	guard, err = store.writer.ValidateBatchApprovalDispatchGraph(t.Context(), parentID, parentID+"-event")
	if err != nil {
		t.Fatalf("validate batch approval dispatch graph: %v", err)
	}
	return parentID, childID, eventID, guard
}

func seedBatchApprovalFailedRetry(
	t *testing.T,
	store approvalAtomicBehaviorStore,
	suffix string,
) (parentID, childID, eventID string) {
	t.Helper()
	parentID = "batch-retry-" + suffix + "-parent"
	childID = "batch-retry-" + suffix + "-child"
	eventID = childID + "-event"
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     parentID,
		parentStatus: "FAILED",
		parentEvent:  "FAILED",
		batchStatus:  "FAILED",
		children: []batchApprovalDispatchChild{{
			ticketID:     childID,
			eventID:      eventID,
			ticketState:  "FAILED",
			eventState:   "FAILED",
			attemptCount: 1,
		}},
	})
	return parentID, childID, eventID
}

func installBatchApprovalSafetyTrigger(t *testing.T, store approvalAtomicBehaviorStore, script string) {
	t.Helper()
	if _, err := store.pool.Exec(t.Context(), script); err != nil {
		t.Fatalf("install batch approval safety trigger: %v", err)
	}
}

func marshalBatchApprovalSafetyPayload(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal batch approval safety payload: %v", err)
	}
	return payload
}
