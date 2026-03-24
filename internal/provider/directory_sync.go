package provider

import (
	"time"

	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
)

// DirectoryConflictCode identifies canonical pre-persistence conflict classes.
type DirectoryConflictCode = directorycontract.DirectoryConflictCode

const (
	DirectoryConflictSameExternalIdentity = directorycontract.DirectoryConflictSameExternalIdentity
	DirectoryConflictUsernameConflict     = directorycontract.DirectoryConflictUsernameConflict
	DirectoryConflictEmailConflict        = directorycontract.DirectoryConflictEmailConflict
	DirectoryConflictAmbiguousExisting    = directorycontract.DirectoryConflictAmbiguousExisting
)

// DirectoryAction identifies the canonical create/update/blocked result semantics
// shared by preview and execution summaries.
type DirectoryAction = directorycontract.DirectoryAction

const (
	DirectoryActionCreate  = directorycontract.DirectoryActionCreate
	DirectoryActionUpdate  = directorycontract.DirectoryActionUpdate
	DirectoryActionBlocked = directorycontract.DirectoryActionBlocked
)

// DirectoryPreviewMatchBy identifies the canonical safe-match anchor used by preview.
type DirectoryPreviewMatchBy = directorycontract.DirectoryPreviewMatchBy

const (
	DirectoryPreviewMatchByExternalID = directorycontract.DirectoryPreviewMatchByExternalID
)

// DirectorySyncDescriptor describes provider-owned directory sync input.
type DirectorySyncDescriptor = directorycontract.DirectorySyncDescriptor

// DirectoryUserRecord is the canonical directory import record consumed by core.
type DirectoryUserRecord = directorycontract.DirectoryUserRecord

// DirectoryConflict captures canonical conflict classification details.
type DirectoryConflict = directorycontract.DirectoryConflict

// DirectoryPreviewMatch captures the canonical apply action and safe-match anchor.
type DirectoryPreviewMatch = directorycontract.DirectoryPreviewMatch

// DirectoryActionSummary captures action-count totals for preview/result
// aggregation without reintroducing provider-specific flow semantics.
type DirectoryActionSummary = directorycontract.DirectoryActionSummary

// DirectoryPreviewItem is the canonical preview row returned to admin clients.
type DirectoryPreviewItem = directorycontract.DirectoryPreviewItem

// DirectorySyncPreview is the provider-agnostic preview response contract.
type DirectorySyncPreview = directorycontract.DirectorySyncPreview

// DirectoryEnrichmentMode identifies the canonical scheduled enrichment mode.
type DirectoryEnrichmentMode = directorycontract.DirectoryEnrichmentMode

const (
	DirectoryEnrichmentModeEnrichExistingOnly = directorycontract.DirectoryEnrichmentModeEnrichExistingOnly
)

// DirectoryJoinKeyType identifies the explicit join rule used by scheduled enrichment.
type DirectoryJoinKeyType = directorycontract.DirectoryJoinKeyType

const (
	DirectoryJoinKeyUsername = directorycontract.DirectoryJoinKeyUsername
)

// ScheduledDirectoryEnrichmentPlan is the provider-owned plan consumed by the
// core scheduler.
type ScheduledDirectoryEnrichmentPlan = directorycontract.ScheduledDirectoryEnrichmentPlan

// DirectorySyncCapability is an optional auth-provider admin extension.
type DirectorySyncCapability = directorycontract.DirectorySyncCapability

// ScheduledDirectoryEnrichmentCapability is an optional provider-owned scheduler plan.
type ScheduledDirectoryEnrichmentCapability = directorycontract.ScheduledDirectoryEnrichmentCapability

// DirectorySyncRequestError indicates provider_request validation failure.
type DirectorySyncRequestError = directorycontract.DirectorySyncRequestError

// NormalizeScheduledDirectoryEnrichmentPlan validates and defaults a provider-owned
// scheduled enrichment plan into the canonical core shape.
func NormalizeScheduledDirectoryEnrichmentPlan(
	plan *ScheduledDirectoryEnrichmentPlan,
) (*ScheduledDirectoryEnrichmentPlan, *time.Location, error) {
	return directorycontract.NormalizeScheduledDirectoryEnrichmentPlan(plan)
}

// NewDirectorySyncRequestError constructs a request-validation error.
func NewDirectorySyncRequestError(message string) error {
	return directorycontract.NewDirectorySyncRequestError(message)
}

// CloneDirectoryAttributes clones an opaque JSON-like attribute map.
func CloneDirectoryAttributes(value map[string]interface{}) map[string]interface{} {
	return directorycontract.CloneDirectoryAttributes(value)
}
