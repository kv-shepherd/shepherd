package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/domain"
)

// validateExactBatchPowerGraph proves that the parent approval covered the
// complete immutable child graph. Generic parent type/status checks are not
// sufficient because parent_ticket_id is mutable and two active power batches
// may belong to the same requester.
func validateExactBatchPowerGraph(
	ctx context.Context,
	client *ent.Client,
	currentEventID string,
	parent *ent.Ticket,
	parentEvent *ent.DomainEvent,
	projection *ent.BatchTicket,
) error {
	currentEventID = strings.TrimSpace(currentEventID)
	if client == nil || parent == nil || parentEvent == nil || projection == nil || currentEventID == "" {
		return rejectPowerDispatch(currentEventID, "batch power graph context is incomplete")
	}
	actor := strings.TrimSpace(parent.Requester)
	parentID := strings.TrimSpace(parent.ID)
	if parentID == "" || actor == "" ||
		strings.TrimSpace(parent.ParentTicketID) != "" ||
		parent.OperationType != entticket.OperationTypePOWER ||
		strings.TrimSpace(parent.EventID) == "" ||
		strings.TrimSpace(parent.EventID) != strings.TrimSpace(parentEvent.ID) ||
		strings.TrimSpace(parentEvent.EventType) != string(domain.EventBatchPowerRequested) ||
		strings.TrimSpace(parentEvent.AggregateType) != batchAggregateType ||
		strings.TrimSpace(parentEvent.AggregateID) != parentID ||
		strings.TrimSpace(parentEvent.CreatedBy) != actor ||
		strings.TrimSpace(projection.ID) != parentID ||
		projection.BatchType != batchticket.BatchTypeBATCH_POWER ||
		strings.TrimSpace(projection.CreatedBy) != actor {
		return rejectPowerDispatch(currentEventID, "batch power parent ticket/event/projection identity is inconsistent")
	}

	var parentPayload domain.BatchVMRequestPayload
	if err := decodeVMPowerProvenanceJSON(parentEvent.Payload, &parentPayload); err != nil {
		return rejectPowerDispatch(currentEventID, "batch power parent event payload is malformed")
	}
	if strings.TrimSpace(parentPayload.SubmittedBy) != actor {
		return rejectPowerDispatch(currentEventID, "batch power parent submitter does not match its requester")
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parentID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("load batch power parent %s children: %w", parentID, err)
	}
	if len(children) == 0 || len(children) != len(parentPayload.Items) {
		return rejectPowerDispatch(currentEventID, "batch power parent payload does not match its complete child set")
	}

	eventIDs := make([]string, 0, len(children))
	seenEventIDs := make(map[string]string, len(children))
	for _, child := range children {
		if child == nil || strings.TrimSpace(child.ID) == "" || strings.TrimSpace(child.EventID) == "" {
			return rejectPowerDispatch(currentEventID, "batch power child ticket identity is incomplete")
		}
		if previous, exists := seenEventIDs[child.EventID]; exists {
			return rejectPowerDispatch(
				currentEventID,
				fmt.Sprintf("batch power children %s and %s share event %s", previous, child.ID, child.EventID),
			)
		}
		seenEventIDs[child.EventID] = child.ID
		eventIDs = append(eventIDs, child.EventID)
	}
	events, err := client.DomainEvent.Query().
		Where(domainevent.IDIn(eventIDs...)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("load batch power parent %s child events: %w", parentID, err)
	}
	eventsByID := make(map[string]*ent.DomainEvent, len(events))
	for _, event := range events {
		if event != nil {
			eventsByID[event.ID] = event
		}
	}
	if len(eventsByID) != len(eventIDs) {
		return rejectPowerDispatch(currentEventID, "batch power child event set is incomplete")
	}

	remainingItems := make(map[string]int, len(parentPayload.Items))
	for itemIndex := range parentPayload.Items {
		itemKey, keyErr := batchPowerProvenanceItemKey(parentPayload.Items[itemIndex])
		if keyErr != nil {
			return rejectPowerDispatch(currentEventID, "batch power parent contains an invalid item")
		}
		remainingItems[itemKey]++
	}

	action := ""
	currentFound := false
	seenVMIDs := make(map[string]string, len(children))
	var successCount, failedCount, pendingCount int
	for _, child := range children {
		event := eventsByID[child.EventID]
		if event == nil ||
			strings.TrimSpace(child.ParentTicketID) != parentID ||
			child.OperationType != entticket.OperationTypePOWER ||
			strings.TrimSpace(child.Requester) != actor {
			return rejectPowerDispatch(currentEventID, "batch power child ticket/event identity is inconsistent")
		}
		var payload domain.VMPowerPayload
		if err := decodeVMPowerProvenanceJSON(event.Payload, &payload); err != nil {
			return rejectPowerDispatch(currentEventID, fmt.Sprintf("batch power child event %s payload is malformed", event.ID))
		}
		childAction := strings.ToLower(strings.TrimSpace(payload.Operation))
		if payload.DispatchMode != domain.VMPowerDispatchTicket ||
			(childAction != powerOpStart && childAction != powerOpStop && childAction != powerOpRestart) {
			return rejectPowerDispatch(currentEventID, fmt.Sprintf("batch power child event %s has invalid dispatch provenance", event.ID))
		}
		if err := validateVMPowerEventIdentity(event, payload, childAction); err != nil {
			return rejectPowerDispatch(currentEventID, err.Error())
		}
		if err := validateVMPowerTicketIdentity(child, event.ID, payload); err != nil {
			return rejectPowerDispatch(currentEventID, err.Error())
		}
		if !batchPowerChildStatePairCompatible(child.Status, event.Status) {
			return rejectPowerDispatch(
				currentEventID,
				fmt.Sprintf("batch power child %s ticket/event status pair is inconsistent", child.ID),
			)
		}
		if action == "" {
			action = childAction
		} else if action != childAction {
			return rejectPowerDispatch(currentEventID, "batch power children do not share one approved action")
		}
		vmID := strings.TrimSpace(payload.VMID)
		if previous, exists := seenVMIDs[vmID]; exists {
			return rejectPowerDispatch(
				currentEventID,
				fmt.Sprintf("batch power children %s and %s target the same VM %s", previous, child.ID, vmID),
			)
		}
		seenVMIDs[vmID] = child.ID

		itemKey, keyErr := batchPowerProvenanceItemKey(batchPowerProvenanceItem(payload))
		if keyErr != nil || remainingItems[itemKey] == 0 {
			return rejectPowerDispatch(currentEventID, "batch power parent item set does not match its child events")
		}
		remainingItems[itemKey]--

		if strings.TrimSpace(event.ID) == currentEventID {
			currentFound = true
			childApprover := strings.TrimSpace(child.Approver)
			parentApprover := strings.TrimSpace(parent.Approver)
			if childApprover != parentApprover ||
				(child.Status == entticket.StatusAPPROVED && childApprover == "") {
				return rejectPowerDispatch(currentEventID, "batch power child approver does not match its parent dispatch provenance")
			}
			if child.AttemptCount <= 0 || child.LastAttemptAt == nil || child.LastAttemptAt.IsZero() {
				return rejectPowerDispatch(currentEventID, "batch power child has no durable dispatch-attempt provenance")
			}
		}

		switch child.Status {
		case entticket.StatusSUCCESS:
			successCount++
		case entticket.StatusFAILED, entticket.StatusREJECTED:
			failedCount++
		case entticket.StatusCANCELLED:
		default:
			pendingCount++
		}
	}
	if !currentFound {
		return rejectPowerDispatch(currentEventID, "power event is not owned by the claimed batch parent")
	}
	for _, count := range remainingItems {
		if count != 0 {
			return rejectPowerDispatch(currentEventID, "batch power parent item set does not match its child events")
		}
	}
	if strings.ToUpper(strings.TrimSpace(parentPayload.Operation)) != "POWER_"+strings.ToUpper(action) {
		return rejectPowerDispatch(currentEventID, "batch power parent action does not match its complete child graph")
	}
	if projection.ChildCount != len(children) ||
		projection.SuccessCount != successCount ||
		projection.FailedCount != failedCount ||
		projection.PendingCount != pendingCount {
		return rejectPowerDispatch(currentEventID, "batch power projection counters do not match its complete child graph")
	}
	return nil
}

func validateTerminalVMPowerTicketProvenance(
	ctx context.Context,
	client *ent.Client,
	ticket *ent.Ticket,
	eventStatus domainevent.Status,
	payload domain.VMPowerPayload,
) error {
	if client == nil || ticket == nil {
		return rejectPowerDispatch("", "terminal power ticket provenance context is incomplete")
	}
	eventID := strings.TrimSpace(ticket.EventID)
	parentID := strings.TrimSpace(ticket.ParentTicketID)
	if parentID == "" {
		switch ticket.Status {
		case entticket.StatusPENDING:
			return rejectPowerDispatch(eventID, "terminal standalone power ticket was never approved")
		case entticket.StatusAPPROVED, entticket.StatusEXECUTING, entticket.StatusSUCCESS,
			entticket.StatusFAILED, entticket.StatusREJECTED:
			if strings.TrimSpace(ticket.Approver) == "" {
				return rejectPowerDispatch(eventID, "terminal standalone power ticket has no approval provenance")
			}
		case entticket.StatusCANCELLED:
			// Cancellation does not imply that provider dispatch was approved.
		}
		return nil
	}

	parent, parentEvent, projection, err := lockAndValidateBatchPowerParentGraph(
		ctx,
		client,
		eventID,
		payload,
		parentID,
	)
	if err != nil {
		return err
	}
	if !batchPowerParentStateConsistent(parent, parentEvent, projection) {
		return rejectPowerDispatch(
			eventID,
			fmt.Sprintf("terminal child event status %s has an inconsistent batch parent state", eventStatus),
		)
	}
	return nil
}

func batchPowerParentStateConsistent(
	parent *ent.Ticket,
	parentEvent *ent.DomainEvent,
	projection *ent.BatchTicket,
) bool {
	if parent == nil || parentEvent == nil || projection == nil {
		return false
	}
	switch parent.Status {
	case entticket.StatusEXECUTING:
		return parentEvent.Status == domainevent.StatusPROCESSING &&
			projection.Status == batchticket.StatusIN_PROGRESS &&
			projection.PendingCount > 0
	case entticket.StatusSUCCESS:
		return parentEvent.Status == domainevent.StatusCOMPLETED &&
			projection.Status == batchticket.StatusCOMPLETED &&
			projection.PendingCount == 0 &&
			projection.SuccessCount == projection.ChildCount
	case entticket.StatusFAILED:
		if parentEvent.Status != domainevent.StatusFAILED || projection.PendingCount != 0 {
			return false
		}
		if projection.Status == batchticket.StatusPARTIAL_SUCCESS {
			return projection.SuccessCount > 0 && projection.SuccessCount < projection.ChildCount
		}
		return projection.Status == batchticket.StatusFAILED &&
			projection.SuccessCount == 0 &&
			projection.FailedCount > 0
	case entticket.StatusCANCELLED:
		return parentEvent.Status == domainevent.StatusCANCELLED &&
			projection.Status == batchticket.StatusCANCELLED &&
			projection.PendingCount == 0 &&
			projection.SuccessCount == 0 &&
			projection.FailedCount == 0
	default:
		return false
	}
}

func lockAndValidateBatchPowerParentGraph(
	ctx context.Context,
	client *ent.Client,
	currentEventID string,
	payload domain.VMPowerPayload,
	parentID string,
) (*ent.Ticket, *ent.DomainEvent, *ent.BatchTicket, error) {
	parentID = strings.TrimSpace(parentID)
	parent, err := client.Ticket.Query().
		Where(
			entticket.IDEQ(parentID),
			entticket.ParentTicketIDIsNil(),
			entticket.OperationTypeEQ(entticket.OperationTypePOWER),
			entticket.RequesterEQ(strings.TrimSpace(payload.Actor)),
			lockTicketRowForUpdate(),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil, nil, rejectPowerDispatch(currentEventID, fmt.Sprintf("batch parent ticket %s is missing or inconsistent", parentID))
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("lock batch parent ticket %s: %w", parentID, err)
	}
	parentEvent, err := client.DomainEvent.Query().
		Where(domainevent.IDEQ(strings.TrimSpace(parent.EventID)), lockDomainEventRowForUpdate()).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil, nil, rejectPowerDispatch(currentEventID, fmt.Sprintf("batch parent event for ticket %s is missing", parentID))
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("lock batch parent event for ticket %s: %w", parentID, err)
	}
	projection, err := client.BatchTicket.Query().
		Where(batchticket.IDEQ(parentID), lockBatchTicketRowForUpdate()).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil, nil, rejectPowerDispatch(currentEventID, fmt.Sprintf("batch projection %s is missing", parentID))
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("lock batch projection %s: %w", parentID, err)
	}
	if err := validateExactBatchPowerGraph(ctx, client, currentEventID, parent, parentEvent, projection); err != nil {
		return nil, nil, nil, err
	}
	return parent, parentEvent, projection, nil
}

func persistVMPowerFailureExact(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
	expected ...domainevent.Status,
) error {
	eventID = strings.TrimSpace(eventID)
	vmID := strings.TrimSpace(payload.VMID)
	if client == nil || eventID == "" || vmID == "" || len(expected) == 0 {
		return rejectPowerDispatch(eventID, "exact power failure persistence context is incomplete")
	}
	return withJobsEntTx(ctx, client, func(tx *ent.Tx, txClient *ent.Client) error {
		if err := tx.ExecContext(
			ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			"power:vm:"+vmID,
		); err != nil {
			return fmt.Errorf("lock exact power failure for VM %s: %w", vmID, err)
		}
		tickets, err := txClient.Ticket.Query().
			Where(entticket.EventIDEQ(eventID), lockTicketRowForUpdate()).
			Order(entticket.ByID()).
			Limit(2).
			All(ctx)
		if err != nil {
			return fmt.Errorf("lock ticket binding for failed power event %s: %w", eventID, err)
		}
		lockedEvent, err := txClient.DomainEvent.Query().
			Where(domainevent.IDEQ(eventID), lockDomainEventRowForUpdate()).
			Only(ctx)
		if ent.IsNotFound(err) {
			return rejectPowerDispatch(eventID, "failed power event no longer exists")
		}
		if err != nil {
			return fmt.Errorf("lock failed power event %s: %w", eventID, err)
		}
		if identityErr := validateVMPowerEventIdentity(lockedEvent, payload, operation); identityErr != nil {
			return rejectPowerDispatch(eventID, identityErr.Error())
		}
		if !domainEventStatusIn(lockedEvent.Status, expected) {
			return rejectPowerDispatch(
				eventID,
				fmt.Sprintf("event status %s is outside the exact failure transition", lockedEvent.Status),
			)
		}

		var ticket *ent.Ticket
		switch payload.DispatchMode {
		case domain.VMPowerDispatchDirect:
			if len(tickets) != 0 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("direct mode requires zero tickets, found %d", len(tickets)))
			}
		case domain.VMPowerDispatchTicket:
			if len(tickets) != 1 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("ticket mode requires exactly one ticket, found %d", len(tickets)))
			}
			ticket = tickets[0]
			if identityErr := validateVMPowerTicketIdentity(ticket, eventID, payload); identityErr != nil {
				return rejectPowerDispatch(eventID, identityErr.Error())
			}
			if authorizationErr := validatePowerFailureTicketAuthorization(ctx, txClient, ticket, lockedEvent, payload); authorizationErr != nil {
				return authorizationErr
			}
		default:
			return rejectPowerDispatch(eventID, fmt.Sprintf("invalid dispatch mode %q", payload.DispatchMode))
		}

		updated, err := tryUpdateDomainEventStatusWithExpected(
			ctx,
			txClient,
			eventID,
			domainevent.StatusFAILED,
			expected...,
		)
		if err != nil {
			return err
		}
		if !updated {
			return rejectPowerDispatch(eventID, "exact FAILED event transition was not acquired")
		}
		if ticket != nil && ticket.Status != entticket.StatusFAILED {
			affected, err := txClient.Ticket.Update().
				Where(
					entticket.IDEQ(ticket.ID),
					entticket.EventIDEQ(eventID),
					entticket.StatusEQ(ticket.Status),
				).
				SetStatus(entticket.StatusFAILED).
				Save(ctx)
			if err != nil {
				return err
			}
			if affected != 1 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("ticket %s lost its exact failure state", ticket.ID))
			}
		}
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
	})
}

func validatePowerFailureTicketAuthorization(
	ctx context.Context,
	client *ent.Client,
	ticket *ent.Ticket,
	event *ent.DomainEvent,
	payload domain.VMPowerPayload,
) error {
	if client == nil || ticket == nil || event == nil {
		return rejectPowerDispatch("", "power failure authorization context is incomplete")
	}
	eventID := strings.TrimSpace(event.ID)
	if strings.TrimSpace(ticket.ParentTicketID) == "" {
		if strings.TrimSpace(ticket.Approver) == "" {
			return rejectPowerDispatch(eventID, "standalone power ticket has no approval provenance")
		}
		allowed := false
		switch event.Status {
		case domainevent.StatusPENDING:
			allowed = ticket.Status == entticket.StatusAPPROVED
		case domainevent.StatusPROCESSING:
			allowed = ticket.Status == entticket.StatusEXECUTING
		case domainevent.StatusFAILED:
			allowed = ticket.Status == entticket.StatusAPPROVED ||
				ticket.Status == entticket.StatusEXECUTING ||
				ticket.Status == entticket.StatusFAILED
		case domainevent.StatusCOMPLETED, domainevent.StatusCANCELLED:
			// Terminal events are intentionally ineligible for failure convergence.
		}
		if !allowed {
			return rejectPowerDispatch(eventID, "standalone ticket/event state is not eligible for exact failure convergence")
		}
		return nil
	}

	if !batchPowerChildStatePairCompatible(ticket.Status, event.Status) {
		return rejectPowerDispatch(eventID, "batch child ticket/event state is not eligible for exact failure convergence")
	}
	parent, parentEvent, projection, err := lockAndValidateBatchPowerParentGraph(
		ctx,
		client,
		eventID,
		payload,
		ticket.ParentTicketID,
	)
	if err != nil {
		return err
	}
	if !batchPowerParentStateConsistent(parent, parentEvent, projection) {
		return rejectPowerDispatch(eventID, "batch parent state is inconsistent with exact failure convergence")
	}
	return nil
}

func domainEventStatusIn(status domainevent.Status, allowed []domainevent.Status) bool {
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
}

func batchPowerChildStatePairCompatible(ticketStatus entticket.Status, eventStatus domainevent.Status) bool {
	switch eventStatus {
	case domainevent.StatusPENDING:
		return ticketStatus == entticket.StatusPENDING ||
			ticketStatus == entticket.StatusAPPROVED ||
			ticketStatus == entticket.StatusEXECUTING
	case domainevent.StatusPROCESSING:
		return ticketStatus == entticket.StatusEXECUTING
	case domainevent.StatusCOMPLETED, domainevent.StatusFAILED, domainevent.StatusCANCELLED:
		target, _ := ticketStatusForTerminalDomainEvent(eventStatus)
		_, compatible := terminalPowerTicketRepairDecision(ticketStatus, target)
		return compatible
	default:
		return false
	}
}

func decodeVMPowerProvenanceJSON(payload []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("payload contains multiple JSON values")
		}
		return err
	}
	return nil
}

func batchPowerProvenanceItem(payload domain.VMPowerPayload) domain.BatchVMItemPayload {
	return domain.BatchVMItemPayload{
		VMID:               payload.VMID,
		VMName:             payload.VMName,
		SystemID:           payload.SystemID,
		SystemName:         payload.SystemName,
		ServiceID:          payload.ServiceID,
		ServiceName:        payload.ServiceName,
		TemplateID:         payload.TemplateID,
		TemplateName:       payload.TemplateName,
		InstanceSizeID:     payload.InstanceSizeID,
		InstanceSizeName:   payload.InstanceSizeName,
		Namespace:          payload.Namespace,
		ClusterID:          payload.ClusterID,
		ClusterName:        payload.ClusterName,
		ClusterEnvironment: payload.ClusterEnvironment,
		OwnerID:            payload.OwnerID,
		OwnerDisplayName:   payload.OwnerDisplayName,
		OwnerUsername:      payload.OwnerUsername,
		RequestVMStatus:    payload.RequestVMStatus,
		CurrentCPUCores:    payload.CurrentCPUCores,
		CurrentMemoryGi:    payload.CurrentMemoryGi,
		CurrentDiskGB:      payload.CurrentDiskGB,
		Operation:          strings.ToLower(strings.TrimSpace(payload.Operation)),
	}
}

func batchPowerProvenanceItemKey(item domain.BatchVMItemPayload) (string, error) {
	item.VMID = strings.TrimSpace(item.VMID)
	item.VMName = strings.TrimSpace(item.VMName)
	item.SystemID = strings.TrimSpace(item.SystemID)
	item.SystemName = strings.TrimSpace(item.SystemName)
	item.ServiceID = strings.TrimSpace(item.ServiceID)
	item.ServiceName = strings.TrimSpace(item.ServiceName)
	item.TemplateID = strings.TrimSpace(item.TemplateID)
	item.TemplateName = strings.TrimSpace(item.TemplateName)
	item.InstanceSizeID = strings.TrimSpace(item.InstanceSizeID)
	item.InstanceSizeName = strings.TrimSpace(item.InstanceSizeName)
	item.Namespace = strings.TrimSpace(item.Namespace)
	item.ClusterID = strings.TrimSpace(item.ClusterID)
	item.ClusterName = strings.TrimSpace(item.ClusterName)
	item.ClusterEnvironment = strings.TrimSpace(item.ClusterEnvironment)
	item.OwnerID = strings.TrimSpace(item.OwnerID)
	item.OwnerDisplayName = strings.TrimSpace(item.OwnerDisplayName)
	item.OwnerUsername = strings.TrimSpace(item.OwnerUsername)
	item.RequestVMStatus = strings.TrimSpace(item.RequestVMStatus)
	item.Operation = strings.ToLower(strings.TrimSpace(item.Operation))
	// Item-level reasons are presentation/audit input and are not copied into
	// every child event. All execution-relevant and provider fields stay exact.
	item.Reason = ""
	encoded, err := json.Marshal(item)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
