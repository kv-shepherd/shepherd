// Package usecase provides application use cases (Clean Architecture).
//
// ADR-0012: Core approval writes + River enqueue must be atomic in a single pgx.Tx.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/usecase
package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/jobs"
	sqlcrepo "kv-shepherd.io/shepherd/internal/repository/sqlc"
)

// ApprovalAtomicWriter executes approval state transition + River enqueue in one pgx transaction.
type ApprovalAtomicWriter struct {
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
	queries     *sqlcrepo.Queries
}

// NewApprovalAtomicWriter creates a new ADR-0012 atomic writer.
func NewApprovalAtomicWriter(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *ApprovalAtomicWriter {
	return &ApprovalAtomicWriter{
		pool:        pool,
		riverClient: riverClient,
		queries:     sqlcrepo.New(pool),
	}
}

// ApproveCreateAndEnqueue atomically:
// 1) marks ticket APPROVED,
// 2) marks event PROCESSING,
// 3) allocates VM instance/index + inserts VM row,
// 4) inserts River vm_create job via InsertTx.
func (w *ApprovalAtomicWriter) ApproveCreateAndEnqueue(
	ctx context.Context,
	ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID string,
) (vmID, vmName string, err error) {
	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return "", "", fmt.Errorf("approval atomic writer is not initialized")
	}
	if err := w.validateCreateInput(ticketID, eventID, approver, clusterID, serviceID, namespace, requesterID); err != nil {
		return "", "", err
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("begin approval create tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := w.queries.WithTx(tx)

	affected, err := qtx.ApproveCreateTicket(ctx, sqlcrepo.ApproveCreateTicketParams{
		Approver:             pgtype.Text{String: approver, Valid: true},
		SelectedClusterID:    pgtype.Text{String: clusterID, Valid: true},
		SelectedStorageClass: strings.TrimSpace(storageClass),
		ID:                   ticketID,
		EventID:              eventID,
	})
	if err != nil {
		return "", "", fmt.Errorf("approve create ticket %s: %w", ticketID, err)
	}
	if affected == 0 {
		return "", "", fmt.Errorf("approve create ticket %s: not pending or operation type mismatch", ticketID)
	}

	affected, err = qtx.SetDomainEventStatus(ctx, sqlcrepo.SetDomainEventStatusParams{
		ID:     eventID,
		Status: "PROCESSING",
	})
	if err != nil {
		return "", "", fmt.Errorf("set event %s to PROCESSING: %w", eventID, err)
	}
	if affected == 0 {
		return "", "", fmt.Errorf("domain event %s not found", eventID)
	}

	allocated, err := qtx.AllocateServiceInstance(ctx, serviceID)
	if err != nil {
		return "", "", fmt.Errorf("allocate service instance for service %s: %w", serviceID, err)
	}

	instance := fmt.Sprintf("%02d", allocated.AllocatedIndex)
	vmName = fmt.Sprintf("%s-%s-%s-%s", namespace, allocated.SystemName, allocated.ServiceName, instance)

	vmUUID, err := uuid.NewV7()
	if err != nil {
		return "", "", fmt.Errorf("generate vm id: %w", err)
	}
	vmID = vmUUID.String()

	if err := qtx.InsertVM(ctx, sqlcrepo.InsertVMParams{
		ID:         vmID,
		Name:       vmName,
		Instance:   instance,
		Namespace:  namespace,
		ClusterID:  pgtype.Text{String: clusterID, Valid: true},
		Hostname:   pgtype.Text{String: vmName, Valid: true},
		CreatedBy:  requesterID,
		TicketID:   pgtype.Text{String: ticketID, Valid: true},
		ServiceVms: serviceID,
	}); err != nil {
		return "", "", fmt.Errorf("insert vm %s: %w", vmID, err)
	}

	if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMCreateArgs{
		EventID: eventID,
	}, nil); err != nil {
		return "", "", fmt.Errorf("enqueue vm_create for event %s: %w", eventID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("commit approval create tx: %w", err)
	}

	return vmID, vmName, nil
}

// ApproveDeleteAndEnqueue atomically:
// 1) marks ticket APPROVED,
// 2) marks event PROCESSING,
// 3) marks VM DELETING (best effort),
// 4) inserts River vm_delete job via InsertTx.
func (w *ApprovalAtomicWriter) ApproveDeleteAndEnqueue(
	ctx context.Context,
	ticketID, eventID, approver, vmID string,
) error {
	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if strings.TrimSpace(ticketID) == "" || strings.TrimSpace(eventID) == "" || strings.TrimSpace(approver) == "" {
		return fmt.Errorf("approve delete input is incomplete")
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin approval delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := w.queries.WithTx(tx)

	affected, err := qtx.ApproveDeleteTicket(ctx, sqlcrepo.ApproveDeleteTicketParams{
		Approver: pgtype.Text{String: approver, Valid: true},
		ID:       ticketID,
		EventID:  eventID,
	})
	if err != nil {
		return fmt.Errorf("approve delete ticket %s: %w", ticketID, err)
	}
	if affected == 0 {
		return fmt.Errorf("approve delete ticket %s: not pending or operation type mismatch", ticketID)
	}

	affected, err = qtx.SetDomainEventStatus(ctx, sqlcrepo.SetDomainEventStatusParams{
		ID:     eventID,
		Status: "PROCESSING",
	})
	if err != nil {
		return fmt.Errorf("set event %s to PROCESSING: %w", eventID, err)
	}
	if affected == 0 {
		return fmt.Errorf("domain event %s not found", eventID)
	}

	if strings.TrimSpace(vmID) != "" {
		if _, err := qtx.SetVMStatus(ctx, sqlcrepo.SetVMStatusParams{
			ID:     vmID,
			Status: "DELETING",
		}); err != nil {
			return fmt.Errorf("set vm %s status to DELETING: %w", vmID, err)
		}
	}

	if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMDeleteArgs{
		EventID: eventID,
	}, nil); err != nil {
		return fmt.Errorf("enqueue vm_delete for event %s: %w", eventID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit approval delete tx: %w", err)
	}
	return nil
}

func (w *ApprovalAtomicWriter) validateCreateInput(
	ticketID, eventID, approver, clusterID, serviceID, namespace, requesterID string,
) error {
	switch {
	case strings.TrimSpace(ticketID) == "":
		return fmt.Errorf("ticket id is required")
	case strings.TrimSpace(eventID) == "":
		return fmt.Errorf("event id is required")
	case strings.TrimSpace(approver) == "":
		return fmt.Errorf("approver is required")
	case strings.TrimSpace(clusterID) == "":
		return fmt.Errorf("selected cluster is required for create approval")
	case strings.TrimSpace(serviceID) == "":
		return fmt.Errorf("service id is required")
	case strings.TrimSpace(namespace) == "":
		return fmt.Errorf("namespace is required")
	case strings.TrimSpace(requesterID) == "":
		return fmt.Errorf("requester id is required")
	default:
		return nil
	}
}
