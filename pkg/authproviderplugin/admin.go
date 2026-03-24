package authproviderplugin

import (
	"fmt"

	admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
)

// AdminSampleField is the standardized sample-field contract for auth-provider plugins.
type AdminSampleField = admincontract.AuthProviderSampleField

// AdminTypeDescriptor is the discoverable plugin type metadata returned by registry/API.
type AdminTypeDescriptor = admincontract.AuthProviderTypeDescriptor

// DirectorySyncDescriptor describes provider-owned directory sync input.
type DirectorySyncDescriptor = directorycontract.DirectorySyncDescriptor

// DirectoryUserRecord is the canonical directory import record consumed by core.
type DirectoryUserRecord = directorycontract.DirectoryUserRecord

// DirectoryConflictCode identifies canonical pre-persistence conflict classes.
type DirectoryConflictCode = directorycontract.DirectoryConflictCode

const (
	// DirectoryConflictSameExternalIdentity means the same provider/external_id already exists.
	DirectoryConflictSameExternalIdentity = directorycontract.DirectoryConflictSameExternalIdentity
	// DirectoryConflictUsernameConflict means username is already owned by another user.
	DirectoryConflictUsernameConflict = directorycontract.DirectoryConflictUsernameConflict
	// DirectoryConflictEmailConflict means email is already owned by another user.
	DirectoryConflictEmailConflict = directorycontract.DirectoryConflictEmailConflict
	// DirectoryConflictAmbiguousExisting means an existing user cannot be linked safely.
	DirectoryConflictAmbiguousExisting = directorycontract.DirectoryConflictAmbiguousExisting
)

// DirectoryConflict captures canonical conflict classification details.
type DirectoryConflict = directorycontract.DirectoryConflict

// DirectoryAction identifies the canonical create/update/blocked semantics shared
// by preview and execution summaries.
type DirectoryAction = directorycontract.DirectoryAction

const (
	// DirectoryActionCreate means the record would create a new canonical user.
	DirectoryActionCreate = directorycontract.DirectoryActionCreate
	// DirectoryActionUpdate means the record would safely update an existing canonical user.
	DirectoryActionUpdate = directorycontract.DirectoryActionUpdate
	// DirectoryActionBlocked means the record cannot be applied safely without resolution.
	DirectoryActionBlocked = directorycontract.DirectoryActionBlocked
)

// DirectoryPreviewMatchBy identifies the canonical safe-match anchor used by preview.
type DirectoryPreviewMatchBy = directorycontract.DirectoryPreviewMatchBy

const (
	// DirectoryPreviewMatchByExternalID means the preview safely matched by provider/external_id.
	DirectoryPreviewMatchByExternalID = directorycontract.DirectoryPreviewMatchByExternalID
)

// DirectoryPreviewMatch captures the canonical apply action and safe-match anchor.
type DirectoryPreviewMatch = directorycontract.DirectoryPreviewMatch

// DirectoryActionSummary captures canonical action-count totals.
type DirectoryActionSummary = directorycontract.DirectoryActionSummary

// DirectoryPreviewItem is the canonical preview row returned to admin clients.
type DirectoryPreviewItem = directorycontract.DirectoryPreviewItem

// DirectorySyncPreview is the provider-agnostic preview response contract.
type DirectorySyncPreview = directorycontract.DirectorySyncPreview

// DirectoryEnrichmentMode identifies the canonical scheduled enrichment mode.
type DirectoryEnrichmentMode = directorycontract.DirectoryEnrichmentMode

const (
	// DirectoryEnrichmentModeEnrichExistingOnly enriches only already-known canonical users.
	DirectoryEnrichmentModeEnrichExistingOnly = directorycontract.DirectoryEnrichmentModeEnrichExistingOnly
)

// DirectoryJoinKeyType identifies the explicit join rule used by scheduled enrichment.
type DirectoryJoinKeyType = directorycontract.DirectoryJoinKeyType

const (
	// DirectoryJoinKeyUsername matches external records to canonical users by username.
	DirectoryJoinKeyUsername = directorycontract.DirectoryJoinKeyUsername
)

// ScheduledDirectoryEnrichmentPlan is the provider-owned plan consumed by the core scheduler.
type ScheduledDirectoryEnrichmentPlan = directorycontract.ScheduledDirectoryEnrichmentPlan

// DirectorySyncRequestError indicates provider_request validation failure.
type DirectorySyncRequestError = directorycontract.DirectorySyncRequestError

// NewDirectorySyncRequestError constructs a request-validation error.
func NewDirectorySyncRequestError(message string) error {
	return directorycontract.NewDirectorySyncRequestError(message)
}

// AdminAdapter is the admin-side plugin contract.
type AdminAdapter = admincontract.AuthProviderAdminAdapter

// AdminAdapterDescriber allows plugins to expose type metadata and config schema.
type AdminAdapterDescriber = admincontract.AuthProviderAdminAdapterDescriber

// DirectorySyncCapability is an optional auth-provider admin extension.
type DirectorySyncCapability = directorycontract.DirectorySyncCapability

// ScheduledDirectoryEnrichmentCapability is an optional provider-owned scheduler plan.
type ScheduledDirectoryEnrichmentCapability = directorycontract.ScheduledDirectoryEnrichmentCapability

// RegisterAdminAdapter registers a provider plugin adapter.
func RegisterAdminAdapter(adapter AdminAdapter) error {
	return adminglobal.Register(adapter)
}

// MustRegisterAdminAdapter registers a provider plugin adapter and panics on failure.
func MustRegisterAdminAdapter(adapter AdminAdapter) {
	if err := RegisterAdminAdapter(adapter); err != nil {
		panic(fmt.Sprintf("auth provider plugin register failed: %v", err))
	}
}

// ListRegisteredAdminTypes returns current registered plugin types.
func ListRegisteredAdminTypes() []AdminTypeDescriptor {
	return adminglobal.List()
}
