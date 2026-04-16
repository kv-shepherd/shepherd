package handlers

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/domain"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

var activeVMOperationTicketStatuses = []entticket.Status{
	entticket.StatusPENDING,
	entticket.StatusAPPROVED,
	entticket.StatusEXECUTING,
}

func (s *Server) findLatestActiveVMTicket(
	ctx context.Context,
	vmID string,
	operationType entticket.OperationType,
	eventTypes ...domain.EventType,
) (*ent.Ticket, error) {
	normalizedVMID := strings.TrimSpace(vmID)
	if normalizedVMID == "" {
		return nil, nil
	}

	eventQuery := s.client.DomainEvent.Query().Where(
		domainevent.AggregateTypeEQ("vm"),
		domainevent.AggregateIDEQ(normalizedVMID),
	)
	if len(eventTypes) > 0 {
		eventTypeValues := make([]string, 0, len(eventTypes))
		for _, eventType := range eventTypes {
			eventTypeValues = append(eventTypeValues, string(eventType))
		}
		eventQuery = eventQuery.Where(domainevent.EventTypeIn(eventTypeValues...))
	}

	eventIDs, err := eventQuery.Select(domainevent.FieldID).Strings(ctx)
	if err != nil {
		return nil, err
	}
	if len(eventIDs) == 0 {
		return nil, nil
	}

	ticket, err := s.client.Ticket.Query().
		Where(
			entticket.EventIDIn(eventIDs...),
			entticket.OperationTypeEQ(operationType),
			entticket.StatusIn(activeVMOperationTicketStatuses...),
		).
		Order(ent.Desc(entticket.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return ticket, nil
}

func writeDuplicatePendingVMOperation(c *gin.Context, existingTicket *ent.Ticket) {
	appErr := apperrors.Conflict(
		apperrors.CodeDuplicateRequest,
		"an active VM operation request already exists for this resource",
	).WithParams(map[string]interface{}{
		"existing_ticket_id": existingTicket.ID,
	})
	c.JSON(appErr.HTTPStatus, toGeneratedError(appErr))
}
