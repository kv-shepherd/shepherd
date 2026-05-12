// Package usecase contains a clean Job Args struct with only allowed fields.
package usecase

// CreateVMJobArgs is a compliant River Job Args struct.
type CreateVMJobArgs struct {
	EventID string // allowed: claim check via EventID
	JobID   string // allowed: claim check via an owning job table
	BatchID string // allowed: for batch operations
	TraceID string // allowed: correlation only
}
