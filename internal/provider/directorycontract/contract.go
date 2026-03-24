package directorycontract

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
)

// ExternalCohort is the provider-agnostic external organization shape reused
// by directory preview/import contracts.
type ExternalCohort = runtimecontract.ExternalCohort

// DirectoryConflictCode identifies canonical pre-persistence conflict classes.
type DirectoryConflictCode string

const (
	DirectoryConflictSameExternalIdentity DirectoryConflictCode = "same_external_identity"
	DirectoryConflictUsernameConflict     DirectoryConflictCode = "username_conflict"
	DirectoryConflictEmailConflict        DirectoryConflictCode = "email_conflict"
	DirectoryConflictAmbiguousExisting    DirectoryConflictCode = "ambiguous_existing_user"
)

// DirectoryAction identifies the canonical create/update/blocked result semantics
// shared by preview and execution summaries.
type DirectoryAction string

const (
	// DirectoryActionCreate means the record would create a new canonical user.
	DirectoryActionCreate DirectoryAction = "create"
	// DirectoryActionUpdate means the record would safely update an existing canonical user.
	DirectoryActionUpdate DirectoryAction = "update"
	// DirectoryActionBlocked means the record cannot be applied safely without resolution.
	DirectoryActionBlocked DirectoryAction = "blocked"
)

// DirectoryPreviewMatchBy identifies the canonical safe-match anchor used by preview.
type DirectoryPreviewMatchBy string

const (
	// DirectoryPreviewMatchByExternalID means the preview safely matched by provider/external_id.
	DirectoryPreviewMatchByExternalID DirectoryPreviewMatchBy = "external_id"
)

// DirectorySyncDescriptor describes provider-owned directory sync input.
type DirectorySyncDescriptor struct {
	DisplayName     string                 `json:"display_name"`
	Description     string                 `json:"description,omitempty"`
	RequestSchema   map[string]interface{} `json:"request_schema,omitempty"`
	SupportsPreview bool                   `json:"supports_preview"`
}

// DirectoryUserRecord is the canonical directory import record consumed by core.
type DirectoryUserRecord struct {
	ExternalID  string                 `json:"external_id"`
	Username    string                 `json:"username"`
	DisplayName string                 `json:"display_name"`
	Email       string                 `json:"email,omitempty"`
	Cohorts     []ExternalCohort       `json:"cohorts,omitempty"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
}

// DirectoryConflict captures canonical conflict classification details.
type DirectoryConflict struct {
	Code           DirectoryConflictCode `json:"code"`
	Field          string                `json:"field,omitempty"`
	ExistingUserID string                `json:"existing_user_id,omitempty"`
	Message        string                `json:"message,omitempty"`
}

// DirectoryPreviewMatch captures the canonical apply action and safe-match anchor.
type DirectoryPreviewMatch struct {
	Action         DirectoryAction         `json:"action"`
	ExistingUserID string                  `json:"existing_user_id,omitempty"`
	MatchedBy      DirectoryPreviewMatchBy `json:"matched_by,omitempty"`
}

// DirectoryActionSummary captures action-count totals for preview/result
// aggregation without reintroducing provider-specific flow semantics.
type DirectoryActionSummary struct {
	CreateCount  int `json:"create_count"`
	UpdateCount  int `json:"update_count"`
	BlockedCount int `json:"blocked_count"`
}

// Add increments the canonical action bucket.
func (s *DirectoryActionSummary) Add(action DirectoryAction) {
	if s == nil {
		return
	}
	switch action {
	case DirectoryActionCreate:
		s.CreateCount++
	case DirectoryActionUpdate:
		s.UpdateCount++
	case DirectoryActionBlocked:
		s.BlockedCount++
	}
}

// DirectoryPreviewItem is the canonical preview row returned to admin clients.
type DirectoryPreviewItem struct {
	Record    DirectoryUserRecord   `json:"record"`
	Match     DirectoryPreviewMatch `json:"match"`
	Conflicts []DirectoryConflict   `json:"conflicts,omitempty"`
	Warnings  []string              `json:"warnings,omitempty"`
}

// DirectorySyncPreview is the provider-agnostic preview response contract.
type DirectorySyncPreview struct {
	TotalCount int                    `json:"total_count"`
	Items      []DirectoryPreviewItem `json:"items"`
}

// DirectoryEnrichmentMode identifies the canonical scheduled enrichment mode.
type DirectoryEnrichmentMode string

const (
	// DirectoryEnrichmentModeEnrichExistingOnly enriches only already-known canonical users.
	DirectoryEnrichmentModeEnrichExistingOnly DirectoryEnrichmentMode = "enrich_existing_only"
)

// DirectoryJoinKeyType identifies the explicit join rule used by scheduled enrichment.
type DirectoryJoinKeyType string

const (
	// DirectoryJoinKeyUsername matches external records to canonical users by username.
	DirectoryJoinKeyUsername DirectoryJoinKeyType = "username"
)

// ScheduledDirectoryEnrichmentPlan is the provider-owned plan consumed by the
// core scheduler.
type ScheduledDirectoryEnrichmentPlan struct {
	Enabled          bool                    `json:"enabled"`
	Mode             DirectoryEnrichmentMode `json:"mode"`
	JoinKeyType      DirectoryJoinKeyType    `json:"join_key_type"`
	ScheduleCron     string                  `json:"schedule_cron"`
	ScheduleTimezone string                  `json:"schedule_timezone,omitempty"`
	ProviderRequest  map[string]interface{}  `json:"provider_request,omitempty"`
}

// DirectorySyncCapability is an optional auth-provider admin extension.
//
// The capability owns provider request shape and source data acquisition while
// core owns canonical preview/import semantics and conflict handling.
type DirectorySyncCapability interface {
	DescribeDirectorySync() DirectorySyncDescriptor
	PreviewDirectorySync(ctx context.Context, config map[string]interface{}, providerRequest map[string]interface{}) (*DirectorySyncPreview, error)
	ListDirectoryUsers(ctx context.Context, config map[string]interface{}, providerRequest map[string]interface{}) ([]DirectoryUserRecord, error)
}

// ScheduledDirectoryEnrichmentCapability is an optional provider-owned
// scheduler plan for enriching existing users on a schedule.
type ScheduledDirectoryEnrichmentCapability interface {
	BuildScheduledDirectoryEnrichmentPlan(ctx context.Context, config map[string]interface{}) (*ScheduledDirectoryEnrichmentPlan, error)
}

// NormalizeScheduledDirectoryEnrichmentPlan validates and defaults a provider-owned
// scheduled enrichment plan into the canonical core shape.
func NormalizeScheduledDirectoryEnrichmentPlan(
	plan *ScheduledDirectoryEnrichmentPlan,
) (*ScheduledDirectoryEnrichmentPlan, *time.Location, error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("scheduled enrichment plan is nil")
	}

	normalized := *plan
	if !normalized.Enabled {
		return &normalized, time.UTC, nil
	}

	if normalized.Mode == "" {
		normalized.Mode = DirectoryEnrichmentModeEnrichExistingOnly
	}
	if normalized.Mode != DirectoryEnrichmentModeEnrichExistingOnly {
		return nil, nil, fmt.Errorf("unsupported scheduled enrichment mode %q", normalized.Mode)
	}

	if normalized.JoinKeyType == "" {
		normalized.JoinKeyType = DirectoryJoinKeyUsername
	}
	if normalized.JoinKeyType != DirectoryJoinKeyUsername {
		return nil, nil, fmt.Errorf("unsupported scheduled enrichment join key %q", normalized.JoinKeyType)
	}

	normalized.ScheduleCron = strings.TrimSpace(normalized.ScheduleCron)
	if normalized.ScheduleCron == "" {
		return nil, nil, fmt.Errorf("schedule_cron is required when scheduled enrichment is enabled")
	}

	normalized.ScheduleTimezone = strings.TrimSpace(normalized.ScheduleTimezone)
	if normalized.ScheduleTimezone == "" {
		normalized.ScheduleTimezone = "UTC"
	}
	location, err := time.LoadLocation(normalized.ScheduleTimezone)
	if err != nil {
		return nil, nil, fmt.Errorf("load schedule timezone %q: %w", normalized.ScheduleTimezone, err)
	}
	if _, err := cron.ParseStandard(normalized.ScheduleCron); err != nil {
		return nil, nil, fmt.Errorf("parse schedule cron %q: %w", normalized.ScheduleCron, err)
	}

	if len(normalized.ProviderRequest) == 0 {
		normalized.ProviderRequest = map[string]interface{}{}
	} else {
		normalized.ProviderRequest = CloneDirectoryAttributes(normalized.ProviderRequest)
	}
	return &normalized, location, nil
}

// DirectorySyncRequestError indicates provider_request validation failure.
type DirectorySyncRequestError struct {
	Message string
}

func (e *DirectorySyncRequestError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// NewDirectorySyncRequestError constructs a request-validation error.
func NewDirectorySyncRequestError(message string) error {
	return &DirectorySyncRequestError{Message: message}
}

// CloneDirectoryAttributes clones an opaque JSON-like attribute map.
func CloneDirectoryAttributes(value map[string]interface{}) map[string]interface{} {
	if len(value) == 0 {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
