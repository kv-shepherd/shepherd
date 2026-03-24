package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/domain"
)

var activeApprovalDeletionGuardStatuses = []entticket.Status{
	entticket.StatusPENDING,
	entticket.StatusAPPROVED,
	entticket.StatusEXECUTING,
}

func (s *Server) countActiveCreateTicketsForServiceIDs(ctx context.Context, serviceIDs []string) (int, error) {
	if s == nil || s.client == nil {
		return 0, nil
	}

	seen := make(map[string]struct{}, len(serviceIDs))
	filteredIDs := make([]string, 0, len(serviceIDs))
	for _, rawID := range serviceIDs {
		serviceID := strings.TrimSpace(rawID)
		if serviceID == "" {
			continue
		}
		if _, ok := seen[serviceID]; ok {
			continue
		}
		seen[serviceID] = struct{}{}
		filteredIDs = append(filteredIDs, serviceID)
	}
	if len(filteredIDs) == 0 {
		return 0, nil
	}

	eventIDs, err := s.client.DomainEvent.Query().
		Where(
			domainevent.EventTypeEQ(string(domain.EventVMCreationRequested)),
			domainevent.AggregateTypeEQ("vm"),
			domainevent.AggregateIDIn(filteredIDs...),
		).
		IDs(ctx)
	if err != nil || len(eventIDs) == 0 {
		return len(eventIDs), err
	}

	return s.client.Ticket.Query().
		Where(
			entticket.OperationTypeEQ(entticket.OperationTypeCREATE),
			entticket.EventIDIn(eventIDs...),
			entticket.StatusIn(activeApprovalDeletionGuardStatuses...),
		).
		Count(ctx)
}

func (s *Server) countActiveCreateTicketsForCluster(ctx context.Context, clusterID string) (int, error) {
	if s == nil || s.client == nil {
		return 0, nil
	}

	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return 0, nil
	}

	return s.client.Ticket.Query().
		Where(
			entticket.OperationTypeEQ(entticket.OperationTypeCREATE),
			entticket.SelectedClusterIDEQ(clusterID),
			entticket.StatusIn(activeApprovalDeletionGuardStatuses...),
		).
		Count(ctx)
}

func (s *Server) countActiveCreateTicketsForTemplate(ctx context.Context, templateID string) (int, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return 0, nil
	}
	return s.countActiveCreateTicketsMatchingPayload(ctx, func(payload domain.VMCreationPayload) bool {
		return strings.TrimSpace(payload.TemplateID) == templateID
	})
}

func (s *Server) countActiveCreateTicketsForInstanceSize(ctx context.Context, instanceSizeID string) (int, error) {
	instanceSizeID = strings.TrimSpace(instanceSizeID)
	if instanceSizeID == "" {
		return 0, nil
	}
	return s.countActiveCreateTicketsMatchingPayload(ctx, func(payload domain.VMCreationPayload) bool {
		return strings.TrimSpace(payload.InstanceSizeID) == instanceSizeID
	})
}

func (s *Server) countActiveCreateTicketsForNamespace(ctx context.Context, namespace string) (int, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return 0, nil
	}
	return s.countActiveCreateTicketsMatchingPayload(ctx, func(payload domain.VMCreationPayload) bool {
		return strings.TrimSpace(payload.Namespace) == namespace
	})
}

func (s *Server) countActiveCreateTicketsMatchingPayload(
	ctx context.Context,
	match func(domain.VMCreationPayload) bool,
) (int, error) {
	if s == nil || s.client == nil || match == nil {
		return 0, nil
	}

	tickets, err := s.client.Ticket.Query().
		Where(
			entticket.OperationTypeEQ(entticket.OperationTypeCREATE),
			entticket.StatusIn(activeApprovalDeletionGuardStatuses...),
		).
		All(ctx)
	if err != nil || len(tickets) == 0 {
		return len(tickets), err
	}

	eventIDs := make([]string, 0, len(tickets))
	seenEventIDs := make(map[string]struct{}, len(tickets))
	for _, ticket := range tickets {
		eventID := strings.TrimSpace(ticket.EventID)
		if eventID == "" {
			continue
		}
		if _, ok := seenEventIDs[eventID]; ok {
			continue
		}
		seenEventIDs[eventID] = struct{}{}
		eventIDs = append(eventIDs, eventID)
	}
	if len(eventIDs) == 0 {
		return 0, nil
	}

	events, err := s.client.DomainEvent.Query().
		Where(
			domainevent.IDIn(eventIDs...),
			domainevent.EventTypeEQ(string(domain.EventVMCreationRequested)),
			domainevent.AggregateTypeEQ("vm"),
		).
		All(ctx)
	if err != nil || len(events) == 0 {
		return len(events), err
	}

	eventPayloadByID := make(map[string]domain.VMCreationPayload, len(events))
	for _, event := range events {
		var payload domain.VMCreationPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return 0, err
		}
		eventPayloadByID[event.ID] = payload
	}

	count := 0
	for _, ticket := range tickets {
		payload, ok := eventPayloadByID[ticket.EventID]
		if !ok {
			continue
		}
		if match(payload) {
			count++
		}
	}
	return count, nil
}
