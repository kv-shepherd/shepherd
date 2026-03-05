// Package usecase contains a violation: Job Args with direct business IDs.
package usecase

// BadCreateVMJobArgs violates ADR-0009: contains direct VM and Ticket IDs.
type BadCreateVMJobArgs struct {
	EventID  string
	VMID     string // want `River Job Args BadCreateVMJobArgs contains forbidden field "VMID"`
	TicketID string // want `River Job Args BadCreateVMJobArgs contains forbidden field "TicketID"`
}
