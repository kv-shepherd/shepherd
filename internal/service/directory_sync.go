package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/ent/userdirectoryprofile"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
)

const (
	// DirectoryConflictResolutionSkip skips records that have blocking conflicts.
	DirectoryConflictResolutionSkip = "skip"
	// DirectoryExecutionModeManualImport is the default manual import/sync path.
	DirectoryExecutionModeManualImport = "manual_import"
	// DirectoryExecutionModeScheduledEnrichment enriches existing canonical users on a schedule.
	DirectoryExecutionModeScheduledEnrichment = "scheduled_enrichment"
)

// DirectorySyncApplyResult captures one persistence outcome.
type DirectorySyncApplyResult struct {
	Action      directorycontract.DirectoryAction
	UserID      string
	RBACChanged bool
}

// DirectorySyncService owns canonical conflict classification and persistence.
type DirectorySyncService struct {
	client                 *ent.Client
	roleAssignmentExecutor RoleAssignmentExecutor
}

func NewDirectorySyncService(client *ent.Client) *DirectorySyncService {
	return &DirectorySyncService{client: client}
}

// WithClient derives a service for a different Ent client. Production callers
// that can reconcile managed RoleBindings must use WithTransaction so the
// required user/role row locks share the write transaction.
func (s *DirectorySyncService) WithClient(client *ent.Client) *DirectorySyncService {
	if s == nil {
		return &DirectorySyncService{client: client}
	}
	derived := *s
	derived.client = client
	derived.roleAssignmentExecutor = nil
	return &derived
}

// WithTransaction derives a transaction-scoped directory service and enables
// the role locks required by managed RoleBinding reconciliation.
func (s *DirectorySyncService) WithTransaction(tx *ent.Tx) *DirectorySyncService {
	if tx == nil {
		return s.WithClient(nil)
	}
	derived := s.WithClient(tx.Client())
	derived.roleAssignmentExecutor = tx
	return derived
}

func (s *DirectorySyncService) Preview(
	ctx context.Context,
	authProviderID string,
	records []directorycontract.DirectoryUserRecord,
) (*directorycontract.DirectorySyncPreview, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("directory sync service is not initialized")
	}

	items := make([]directorycontract.DirectoryPreviewItem, 0, len(records))
	for i := range records {
		record := records[i]
		conflicts, err := s.ClassifyRecord(ctx, authProviderID, record)
		if err != nil {
			return nil, err
		}
		items = append(items, directorycontract.DirectoryPreviewItem{
			Record:    normalizeDirectoryRecord(record),
			Match:     directoryPreviewMatch(conflicts),
			Conflicts: conflicts,
		})
	}

	return &directorycontract.DirectorySyncPreview{
		TotalCount: len(items),
		Items:      items,
	}, nil
}

func (s *DirectorySyncService) CanonicalizePreview(
	ctx context.Context,
	authProviderID string,
	preview *directorycontract.DirectorySyncPreview,
) (*directorycontract.DirectorySyncPreview, error) {
	if preview == nil {
		return &directorycontract.DirectorySyncPreview{}, nil
	}
	items := make([]directorycontract.DirectoryPreviewItem, 0, len(preview.Items))
	for i := range preview.Items {
		item := &preview.Items[i]
		conflicts, err := s.ClassifyRecord(ctx, authProviderID, item.Record)
		if err != nil {
			return nil, err
		}
		items = append(items, directorycontract.DirectoryPreviewItem{
			Record:    normalizeDirectoryRecord(item.Record),
			Match:     directoryPreviewMatch(conflicts),
			Conflicts: conflicts,
			Warnings:  append([]string(nil), item.Warnings...),
		})
	}
	totalCount := preview.TotalCount
	if totalCount == 0 {
		totalCount = len(items)
	}
	return &directorycontract.DirectorySyncPreview{
		TotalCount: totalCount,
		Items:      items,
	}, nil
}

func (s *DirectorySyncService) ClassifyRecord(
	ctx context.Context,
	authProviderID string,
	record directorycontract.DirectoryUserRecord,
) ([]directorycontract.DirectoryConflict, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("directory sync service is not initialized")
	}

	record = normalizeDirectoryRecord(record)
	conflicts := make([]directorycontract.DirectoryConflict, 0, 4)

	var sameExternal *ent.User
	if authProviderID != "" && record.ExternalID != "" {
		existing, err := s.client.User.Query().
			Where(
				user.AuthProviderIDEQ(authProviderID),
				user.ExternalIDEQ(record.ExternalID),
			).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return nil, fmt.Errorf("classify same external identity: %w", err)
		}
		if err == nil {
			sameExternal = existing
			conflicts = append(conflicts, directorycontract.DirectoryConflict{
				Code:           directorycontract.DirectoryConflictSameExternalIdentity,
				Field:          "external_id",
				ExistingUserID: existing.ID,
				Message:        "record already linked to an existing user for this provider",
			})
		}
	}

	var usernameUser *ent.User
	var sameCanonicalUser *ent.User
	if record.Username != "" {
		existing, err := s.client.User.Query().
			Where(user.UsernameEQ(record.Username)).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return nil, fmt.Errorf("classify username conflict: %w", err)
		}
		if err == nil {
			usernameUser = existing
			if sameExternal == nil || existing.ID != sameExternal.ID {
				conflicts = append(conflicts, directorycontract.DirectoryConflict{
					Code:           directorycontract.DirectoryConflictUsernameConflict,
					Field:          "username",
					ExistingUserID: existing.ID,
					Message:        "username already belongs to another user",
				})
			}
		}
	}

	var emailUser *ent.User
	if record.Email != "" {
		existing, err := s.client.User.Query().
			Where(user.EmailEQ(record.Email)).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return nil, fmt.Errorf("classify email conflict: %w", err)
		}
		if err == nil {
			emailUser = existing
			if sameExternal == nil || existing.ID != sameExternal.ID {
				conflicts = append(conflicts, directorycontract.DirectoryConflict{
					Code:           directorycontract.DirectoryConflictEmailConflict,
					Field:          "email",
					ExistingUserID: existing.ID,
					Message:        "email already belongs to another user",
				})
			}
		}
	}

	sameCanonicalUser = sameCanonicalIdentityUser(sameExternal, usernameUser, emailUser)
	if sameCanonicalUser != nil {
		filtered := conflicts[:0]
		for _, conflict := range conflicts {
			if conflict.ExistingUserID == sameCanonicalUser.ID &&
				(conflict.Code == directorycontract.DirectoryConflictUsernameConflict ||
					conflict.Code == directorycontract.DirectoryConflictEmailConflict) {
				continue
			}
			filtered = append(filtered, conflict)
		}
		conflicts = filtered
		conflicts = append(conflicts, directorycontract.DirectoryConflict{
			Code:           directorycontract.DirectoryConflictSameCanonicalIdentity,
			Field:          "username,email",
			ExistingUserID: sameCanonicalUser.ID,
			Message:        "record safely matches an existing canonical user by username and email",
		})
	}

	ambiguousUser := ambiguousDirectoryUserCandidate(sameExternal, usernameUser, emailUser)
	if ambiguousUser != nil {
		conflicts = append(conflicts, directorycontract.DirectoryConflict{
			Code:           directorycontract.DirectoryConflictAmbiguousExisting,
			ExistingUserID: ambiguousUser.ID,
			Message:        "existing user matches by non-authoritative fields but cannot be auto-linked safely",
		})
	}

	return dedupeDirectoryConflicts(conflicts), nil
}

func (s *DirectorySyncService) ApplyRecord(
	ctx context.Context,
	authProviderID string,
	record directorycontract.DirectoryUserRecord,
	conflictResolution string,
) (DirectorySyncApplyResult, []directorycontract.DirectoryConflict, error) {
	if s == nil || s.client == nil {
		return DirectorySyncApplyResult{}, nil, fmt.Errorf("directory sync service is not initialized")
	}

	record = normalizeDirectoryRecord(record)
	normalizedConflictResolution := strings.TrimSpace(conflictResolution)
	switch normalizedConflictResolution {
	case "", DirectoryConflictResolutionSkip:
		normalizedConflictResolution = DirectoryConflictResolutionSkip
	default:
		return DirectorySyncApplyResult{}, nil, fmt.Errorf("unsupported directory conflict resolution %q", conflictResolution)
	}

	conflicts, err := s.ClassifyRecord(ctx, authProviderID, record)
	if err != nil {
		return DirectorySyncApplyResult{}, nil, err
	}
	if hasBlockingDirectoryConflicts(conflicts) {
		if normalizedConflictResolution != DirectoryConflictResolutionSkip {
			return DirectorySyncApplyResult{}, conflicts, fmt.Errorf("unsupported directory conflict resolution %q", normalizedConflictResolution)
		}
		return DirectorySyncApplyResult{Action: directorycontract.DirectoryActionBlocked}, conflicts, nil
	}

	externalResult := runtimecontract.AuthResult{
		ProfileAttributes: record.Attributes,
		Cohorts:           record.Cohorts,
	}
	externalAuth := &ExternalAuthService{
		client:                 s.client,
		roleAssignmentExecutor: s.roleAssignmentExecutor,
	}

	targetUserID := directorySameExternalIdentityID(conflicts)
	if targetUserID == "" {
		id, uuidErr := uuid.NewV7()
		if uuidErr != nil {
			return DirectorySyncApplyResult{}, conflicts, fmt.Errorf("generate user id: %w", uuidErr)
		}

		create := s.client.User.Create().
			SetID(id.String()).
			SetUsername(record.Username).
			SetDisplayName(record.DisplayName).
			SetAuthProviderID(authProviderID).
			SetExternalID(record.ExternalID).
			SetEnabled(true)
		if record.Email != "" {
			create = create.SetEmail(record.Email)
		}
		createdUser, createErr := create.Save(ctx)
		if createErr != nil {
			return DirectorySyncApplyResult{}, conflicts, fmt.Errorf("create synced user: %w", createErr)
		}
		if syncErr := externalAuth.syncObservedExternalCohorts(ctx, authProviderID, externalResult.Cohorts); syncErr != nil {
			return DirectorySyncApplyResult{}, conflicts, syncErr
		}
		if profileErr := externalAuth.upsertExternalProfile(ctx, createdUser.ID, externalResult); profileErr != nil {
			return DirectorySyncApplyResult{}, conflicts, profileErr
		}
		rbacChanged, reconcileErr := externalAuth.reconcileExternalCohortRBAC(
			ctx,
			createdUser.ID,
			authProviderID,
			directorySyncRBACCohorts(createdUser.Enabled, externalResult.Cohorts),
		)
		if reconcileErr != nil {
			return DirectorySyncApplyResult{}, conflicts, reconcileErr
		}
		return DirectorySyncApplyResult{Action: directorycontract.DirectoryActionCreate, UserID: createdUser.ID, RBACChanged: rbacChanged}, conflicts, nil
	}

	update := s.client.User.UpdateOneID(targetUserID).
		SetUsername(record.Username).
		SetDisplayName(record.DisplayName).
		SetAuthProviderID(authProviderID).
		SetExternalID(record.ExternalID)
	if record.Email != "" {
		update = update.SetEmail(record.Email)
	} else {
		update = update.ClearEmail()
	}
	updatedUser, updateErr := update.Save(ctx)
	if updateErr != nil {
		return DirectorySyncApplyResult{}, conflicts, fmt.Errorf("update synced user: %w", updateErr)
	}
	if syncErr := externalAuth.syncObservedExternalCohorts(ctx, authProviderID, externalResult.Cohorts); syncErr != nil {
		return DirectorySyncApplyResult{}, conflicts, syncErr
	}
	if profileErr := externalAuth.upsertExternalProfile(ctx, targetUserID, externalResult); profileErr != nil {
		return DirectorySyncApplyResult{}, conflicts, profileErr
	}
	rbacChanged, reconcileErr := externalAuth.reconcileExternalCohortRBAC(
		ctx,
		targetUserID,
		authProviderID,
		directorySyncRBACCohorts(updatedUser.Enabled, externalResult.Cohorts),
	)
	if reconcileErr != nil {
		return DirectorySyncApplyResult{}, conflicts, reconcileErr
	}
	return DirectorySyncApplyResult{Action: directorycontract.DirectoryActionUpdate, UserID: targetUserID, RBACChanged: rbacChanged}, conflicts, nil
}

func (s *DirectorySyncService) ApplyEnrichmentRecord(
	ctx context.Context,
	authProviderID string,
	joinKeyType directorycontract.DirectoryJoinKeyType,
	record directorycontract.DirectoryUserRecord,
) (DirectorySyncApplyResult, error) {
	if s == nil || s.client == nil {
		return DirectorySyncApplyResult{}, fmt.Errorf("directory sync service is not initialized")
	}

	record = normalizeDirectoryRecord(record)
	normalizedJoinKeyType := directorycontract.DirectoryJoinKeyType(strings.TrimSpace(string(joinKeyType)))
	if normalizedJoinKeyType == "" {
		normalizedJoinKeyType = directorycontract.DirectoryJoinKeyUsername
	}

	var matchedUser *ent.User
	switch normalizedJoinKeyType {
	case directorycontract.DirectoryJoinKeyUsername:
		if record.Username == "" {
			return DirectorySyncApplyResult{Action: directorycontract.DirectoryActionBlocked}, nil
		}
		existing, err := s.client.User.Query().
			Where(user.UsernameEQ(record.Username)).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return DirectorySyncApplyResult{Action: directorycontract.DirectoryActionBlocked}, nil
			}
			return DirectorySyncApplyResult{}, fmt.Errorf("match enrichment user by username: %w", err)
		}
		matchedUser = existing
	default:
		return DirectorySyncApplyResult{}, fmt.Errorf("unsupported enrichment join_key_type %q", normalizedJoinKeyType)
	}

	externalResult := runtimecontract.AuthResult{
		ProfileAttributes: record.Attributes,
		Cohorts:           record.Cohorts,
	}
	externalAuth := &ExternalAuthService{
		client:                 s.client,
		roleAssignmentExecutor: s.roleAssignmentExecutor,
	}
	if err := externalAuth.syncObservedExternalCohorts(ctx, authProviderID, externalResult.Cohorts); err != nil {
		return DirectorySyncApplyResult{}, err
	}
	if err := externalAuth.upsertExternalProfile(ctx, matchedUser.ID, externalResult); err != nil {
		return DirectorySyncApplyResult{}, err
	}
	rbacChanged, err := externalAuth.reconcileExternalCohortRBAC(
		ctx,
		matchedUser.ID,
		authProviderID,
		directorySyncRBACCohorts(matchedUser.Enabled, externalResult.Cohorts),
	)
	if err != nil {
		return DirectorySyncApplyResult{}, err
	}

	return DirectorySyncApplyResult{Action: directorycontract.DirectoryActionUpdate, UserID: matchedUser.ID, RBACChanged: rbacChanged}, nil
}

func directorySyncRBACCohorts(userEnabled bool, cohorts []runtimecontract.ExternalCohort) []runtimecontract.ExternalCohort {
	if !userEnabled {
		return nil
	}
	return cohorts
}

func (s *DirectorySyncService) upsertDirectoryProfile(
	ctx context.Context,
	userID string,
	attributes map[string]interface{},
) error {
	attributes = normalizeDirectoryAttributes(attributes)
	existing, err := s.client.UserDirectoryProfile.Query().
		Where(userdirectoryprofile.UserIDEQ(userID)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("query directory profile: %w", err)
	}

	now := time.Now().UTC()
	if err == nil {
		_, updateErr := s.client.UserDirectoryProfile.UpdateOneID(existing.ID).
			SetAttributes(attributes).
			SetLastSyncedAt(now).
			Save(ctx)
		if updateErr != nil {
			return fmt.Errorf("update directory profile: %w", updateErr)
		}
		return nil
	}

	id, uuidErr := uuid.NewV7()
	if uuidErr != nil {
		return fmt.Errorf("generate directory profile id: %w", uuidErr)
	}
	_, createErr := s.client.UserDirectoryProfile.Create().
		SetID(id.String()).
		SetUserID(userID).
		SetAttributes(attributes).
		SetLastSyncedAt(now).
		Save(ctx)
	if createErr != nil {
		return fmt.Errorf("create directory profile: %w", createErr)
	}
	return nil
}

func normalizeDirectoryRecord(record directorycontract.DirectoryUserRecord) directorycontract.DirectoryUserRecord {
	record.ExternalID = normalizeCanonicalUserIdentity(record.ExternalID)
	record.Username = normalizeCanonicalUserIdentity(record.Username)
	record.DisplayName = strings.TrimSpace(record.DisplayName)
	record.Email = strings.TrimSpace(strings.ToLower(record.Email))
	record.Cohorts = normalizeDirectoryCohorts(record.Cohorts)
	record.Attributes = normalizeDirectoryAttributes(record.Attributes)
	return record
}

func normalizeDirectoryCohorts(cohorts []runtimecontract.ExternalCohort) []runtimecontract.ExternalCohort {
	if len(cohorts) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	items := make([]runtimecontract.ExternalCohort, 0, len(cohorts))
	for _, cohort := range cohorts {
		cohort.Kind = strings.TrimSpace(cohort.Kind)
		cohort.Key = strings.TrimSpace(cohort.Key)
		cohort.DisplayName = strings.TrimSpace(cohort.DisplayName)
		if cohort.Kind == "" || cohort.Key == "" {
			continue
		}
		if cohort.DisplayName == "" {
			cohort.DisplayName = cohort.Key
		}
		identity := cohort.Kind + ":" + cohort.Key
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}
		items = append(items, cohort)
	}
	slices.SortFunc(items, func(left, right runtimecontract.ExternalCohort) int {
		if left.Kind != right.Kind {
			return strings.Compare(left.Kind, right.Kind)
		}
		return strings.Compare(left.Key, right.Key)
	})
	if len(items) == 0 {
		return nil
	}
	return items
}

func normalizeDirectoryAttributes(attributes map[string]interface{}) map[string]interface{} {
	if len(attributes) == 0 {
		return map[string]interface{}{}
	}
	normalized := make(map[string]interface{}, len(attributes))
	for key, value := range attributes {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return map[string]interface{}{}
	}
	return normalized
}

func directorySameExternalIdentityID(conflicts []directorycontract.DirectoryConflict) string {
	for _, conflict := range conflicts {
		if conflict.Code == directorycontract.DirectoryConflictSameExternalIdentity ||
			conflict.Code == directorycontract.DirectoryConflictSameCanonicalIdentity {
			return conflict.ExistingUserID
		}
	}
	return ""
}

func directoryPreviewMatch(conflicts []directorycontract.DirectoryConflict) directorycontract.DirectoryPreviewMatch {
	if hasBlockingDirectoryConflicts(conflicts) {
		return directorycontract.DirectoryPreviewMatch{
			Action: directorycontract.DirectoryActionBlocked,
		}
	}
	if existingUserID := directorySameExternalIdentityID(conflicts); existingUserID != "" {
		matchedBy := directorycontract.DirectoryPreviewMatchByExternalID
		for _, conflict := range conflicts {
			switch conflict.Code {
			case directorycontract.DirectoryConflictSameCanonicalIdentity:
				matchedBy = directorycontract.DirectoryPreviewMatchByCanonicalIdentity
			case directorycontract.DirectoryConflictSameExternalIdentity:
				matchedBy = directorycontract.DirectoryPreviewMatchByExternalID
			case directorycontract.DirectoryConflictUsernameConflict,
				directorycontract.DirectoryConflictEmailConflict,
				directorycontract.DirectoryConflictAmbiguousExisting:
				// Blocked conflicts are handled before we attempt to derive a match anchor.
			}
		}
		return directorycontract.DirectoryPreviewMatch{
			Action:         directorycontract.DirectoryActionUpdate,
			ExistingUserID: existingUserID,
			MatchedBy:      matchedBy,
		}
	}
	return directorycontract.DirectoryPreviewMatch{
		Action: directorycontract.DirectoryActionCreate,
	}
}

func hasBlockingDirectoryConflicts(conflicts []directorycontract.DirectoryConflict) bool {
	for _, conflict := range conflicts {
		if conflict.Code != directorycontract.DirectoryConflictSameExternalIdentity &&
			conflict.Code != directorycontract.DirectoryConflictSameCanonicalIdentity {
			return true
		}
	}
	return false
}

func sameCanonicalIdentityUser(sameExternal, usernameUser, emailUser *ent.User) *ent.User {
	if sameExternal != nil {
		return nil
	}
	if usernameUser != nil && emailUser != nil && usernameUser.ID == emailUser.ID {
		return usernameUser
	}
	return nil
}

func ambiguousDirectoryUserCandidate(sameExternal, usernameUser, emailUser *ent.User) *ent.User {
	if sameExternal != nil {
		return nil
	}
	if usernameUser != nil && usernameUser.AuthProviderID == "" && emailUser == nil {
		return usernameUser
	}
	if emailUser != nil && emailUser.AuthProviderID == "" && usernameUser == nil {
		return emailUser
	}
	return nil
}

func dedupeDirectoryConflicts(conflicts []directorycontract.DirectoryConflict) []directorycontract.DirectoryConflict {
	if len(conflicts) == 0 {
		return nil
	}
	deduped := make([]directorycontract.DirectoryConflict, 0, len(conflicts))
	seen := map[string]struct{}{}
	for _, conflict := range conflicts {
		key := string(conflict.Code) + "|" + conflict.Field + "|" + conflict.ExistingUserID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, conflict)
	}
	return deduped
}
