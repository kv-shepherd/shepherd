// Package usecase contains a clean Job Args struct with only allowed fields.
package usecase

// CreateVMJobArgs is a compliant River Job Args struct.
type CreateVMJobArgs struct {
	EventID string // allowed: claim check via EventID
	BatchID string // allowed: for batch operations
}
