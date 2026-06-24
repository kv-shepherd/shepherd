package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohort"
	"kv-shepherd.io/shepherd/ent/user"
	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
)

// ExternalAuthService owns canonical validation and JIT user provisioning for external auth.
type ExternalAuthService struct {
	client *ent.Client
}

func NewExternalAuthService(client *ent.Client) *ExternalAuthService {
	return &ExternalAuthService{client: client}
}

// WithClient derives a transaction-scoped service instance without constructing
// a new dependency graph in request handlers.
func (s *ExternalAuthService) WithClient(client *ent.Client) *ExternalAuthService {
	if s == nil {
		return &ExternalAuthService{client: client}
	}
	derived := *s
	derived.client = client
	return &derived
}

// ExternalAuthUpsertResult captures the canonical persistence outcome.
type ExternalAuthUpsertResult struct {
	User        *ent.User
	Created     bool
	Updated     bool
	RBACChanged bool
}

func (s *ExternalAuthService) UpsertExternalUser(
	ctx context.Context,
	authProviderID string,
	result runtimecontract.AuthResult,
) (*ExternalAuthUpsertResult, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("external auth service is not initialized")
	}
	authProviderID = strings.TrimSpace(authProviderID)
	if authProviderID == "" {
		return nil, fmt.Errorf("auth provider id is required")
	}

	normalized, err := normalizeExternalAuthResult(result)
	if err != nil {
		return nil, err
	}

	return s.upsertExternalUserNormalized(ctx, authProviderID, normalized)
}

func (s *ExternalAuthService) upsertExternalUserNormalized(
	ctx context.Context,
	authProviderID string,
	normalized runtimecontract.AuthResult,
) (*ExternalAuthUpsertResult, error) {
	directoryAuthoritative := normalized.DirectoryAuthority != runtimecontract.AuthDirectoryAuthorityLoginOnly

	existing, err := s.client.User.Query().
		Where(
			user.AuthProviderIDEQ(authProviderID),
			user.ExternalIDEQ(normalized.ExternalID),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, fmt.Errorf("query external user link: %w", err)
	}

	if ent.IsNotFound(err) {
		existing, err = s.findClaimableExistingUser(ctx, normalized)
		if err != nil {
			return nil, err
		}
	}

	if existing == nil {
		if conflictErr := s.ensureExternalIdentityConflicts(ctx, authProviderID, "", normalized); conflictErr != nil {
			return nil, conflictErr
		}

		id, uuidErr := uuid.NewV7()
		if uuidErr != nil {
			return nil, fmt.Errorf("generate user id: %w", uuidErr)
		}

		create := s.client.User.Create().
			SetID(id.String()).
			SetUsername(normalized.Username).
			SetDisplayName(normalized.DisplayName).
			SetAuthProviderID(authProviderID).
			SetExternalID(normalized.ExternalID).
			SetEnabled(normalized.Enabled)
		if normalized.Email != "" {
			create = create.SetEmail(normalized.Email)
		}
		createdUser, createErr := create.Save(ctx)
		if createErr != nil {
			return nil, fmt.Errorf("create external user: %w", createErr)
		}
		if directoryAuthoritative {
			if cohortErr := s.syncObservedExternalCohorts(ctx, authProviderID, normalized.Cohorts); cohortErr != nil {
				return nil, cohortErr
			}
			if profileErr := s.upsertExternalProfile(ctx, createdUser.ID, normalized); profileErr != nil {
				return nil, profileErr
			}
			rbacChanged, reconcileErr := s.reconcileExternalCohortRBAC(ctx, createdUser.ID, authProviderID, externalAuthRBACCohorts(normalized))
			if reconcileErr != nil {
				return nil, reconcileErr
			}
			return &ExternalAuthUpsertResult{
				User:        createdUser,
				Created:     true,
				RBACChanged: rbacChanged,
			}, nil
		}
		return &ExternalAuthUpsertResult{User: createdUser, Created: true}, nil
	}

	if conflictErr := s.ensureExternalIdentityConflicts(ctx, authProviderID, existing.ID, normalized); conflictErr != nil {
		return nil, conflictErr
	}

	preserveExistingDirectoryOwner := !directoryAuthoritative &&
		strings.TrimSpace(existing.AuthProviderID) != "" &&
		existing.AuthProviderID != authProviderID

	update := s.client.User.UpdateOneID(existing.ID).SetEnabled(normalized.Enabled)
	if preserveExistingDirectoryOwner {
		if strings.TrimSpace(existing.DisplayName) == "" && normalized.DisplayName != "" {
			update = update.SetDisplayName(normalized.DisplayName)
		}
		if strings.TrimSpace(existing.Username) == "" && normalized.Username != "" {
			update = update.SetUsername(normalized.Username)
		}
		if strings.TrimSpace(existing.Email) == "" && normalized.Email != "" {
			update = update.SetEmail(normalized.Email)
		}
	} else {
		update = update.
			SetUsername(normalized.Username).
			SetDisplayName(normalized.DisplayName).
			SetAuthProviderID(authProviderID).
			SetExternalID(normalized.ExternalID)
		if normalized.Email != "" {
			update = update.SetEmail(normalized.Email)
		} else {
			update = update.ClearEmail()
		}
	}
	updatedUser, updateErr := update.Save(ctx)
	if updateErr != nil {
		return nil, fmt.Errorf("update external user: %w", updateErr)
	}
	if directoryAuthoritative {
		if cohortErr := s.syncObservedExternalCohorts(ctx, authProviderID, normalized.Cohorts); cohortErr != nil {
			return nil, cohortErr
		}
		if profileErr := s.upsertExternalProfile(ctx, updatedUser.ID, normalized); profileErr != nil {
			return nil, profileErr
		}
		rbacChanged, reconcileErr := s.reconcileExternalCohortRBAC(ctx, updatedUser.ID, authProviderID, externalAuthRBACCohorts(normalized))
		if reconcileErr != nil {
			return nil, reconcileErr
		}
		return &ExternalAuthUpsertResult{
			User:        updatedUser,
			Updated:     true,
			RBACChanged: rbacChanged,
		}, nil
	}
	return &ExternalAuthUpsertResult{
		User:    updatedUser,
		Updated: true,
	}, nil
}

func externalAuthRBACCohorts(result runtimecontract.AuthResult) []runtimecontract.ExternalCohort {
	if !result.Enabled {
		return nil
	}
	return result.Cohorts
}

func (s *ExternalAuthService) findClaimableExistingUser(
	ctx context.Context,
	result runtimecontract.AuthResult,
) (*ent.User, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("external auth service is not initialized")
	}

	candidates := make(map[string]*ent.User, 2)
	loadCandidate := func(query *ent.UserQuery, conflictLabel string) error {
		existing, err := query.Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("query %s claim candidate: %w", conflictLabel, err)
		}
		candidates[existing.ID] = existing
		return nil
	}

	if result.Username != "" {
		if err := loadCandidate(
			s.client.User.Query().Where(user.UsernameEQ(result.Username)),
			"username",
		); err != nil {
			return nil, err
		}
	}
	if result.Email != "" {
		if err := loadCandidate(
			s.client.User.Query().Where(user.EmailEQ(result.Email)),
			"email",
		); err != nil {
			return nil, err
		}
	}

	switch len(candidates) {
	case 0:
		return nil, nil
	case 1:
		var candidate *ent.User
		for _, item := range candidates {
			candidate = item
		}
		if candidate == nil {
			return nil, nil
		}
		if strings.TrimSpace(candidate.PasswordHash) != "" {
			return nil, fmt.Errorf("external identity already belongs to another user")
		}
		return candidate, nil
	default:
		return nil, fmt.Errorf("external identity already belongs to another user")
	}
}

func (s *ExternalAuthService) RecordLogin(ctx context.Context, userID string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("external auth service is not initialized")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user id is required")
	}
	if err := s.client.User.UpdateOneID(userID).SetLastLoginAt(time.Now().UTC()).Exec(ctx); err != nil {
		return fmt.Errorf("update last_login_at: %w", err)
	}
	return nil
}

func (s *ExternalAuthService) ensureExternalIdentityConflicts(
	ctx context.Context,
	authProviderID, currentUserID string,
	result runtimecontract.AuthResult,
) error {
	if result.Username != "" {
		usernameUser, err := s.client.User.Query().
			Where(user.UsernameEQ(result.Username)).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("query username conflict: %w", err)
		}
		if err == nil && usernameUser.ID != currentUserID {
			return fmt.Errorf("username %q already belongs to another user", result.Username)
		}
	}

	if result.Email != "" {
		emailUser, err := s.client.User.Query().
			Where(user.EmailEQ(result.Email)).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("query email conflict: %w", err)
		}
		if err == nil && emailUser.ID != currentUserID {
			return fmt.Errorf("email %q already belongs to another user", result.Email)
		}
	}

	if currentUserID == "" {
		return nil
	}

	externalUser, err := s.client.User.Query().
		Where(
			user.AuthProviderIDEQ(authProviderID),
			user.ExternalIDEQ(result.ExternalID),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("query external id conflict: %w", err)
	}
	if err == nil && externalUser.ID != currentUserID {
		return fmt.Errorf("external id %q already belongs to another user", result.ExternalID)
	}
	return nil
}

func (s *ExternalAuthService) upsertExternalProfile(
	ctx context.Context,
	userID string,
	result runtimecontract.AuthResult,
) error {
	attributes := normalizeDirectoryAttributes(result.ProfileAttributes)
	if len(result.Cohorts) > 0 {
		attributes["external_cohorts"] = externalCohortsToAttributes(result.Cohorts)
	}
	return (&DirectorySyncService{client: s.client}).upsertDirectoryProfile(ctx, userID, attributes)
}

func (s *ExternalAuthService) syncObservedExternalCohorts(
	ctx context.Context,
	authProviderID string,
	cohorts []runtimecontract.ExternalCohort,
) error {
	if s == nil || s.client == nil || strings.TrimSpace(authProviderID) == "" || len(cohorts) == 0 {
		return nil
	}

	now := time.Now().UTC()
	for _, cohort := range normalizeExternalCohorts(cohorts) {
		displayName := strings.TrimSpace(cohort.DisplayName)
		if displayName == "" {
			displayName = cohort.Key
		}
		if err := s.upsertObservedExternalCohort(ctx, authProviderID, cohort.Kind, cohort.Key, displayName, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *ExternalAuthService) upsertObservedExternalCohort(
	ctx context.Context,
	authProviderID, cohortKind, cohortKey, displayName string,
	now time.Time,
) error {
	existing, err := s.client.ExternalCohort.Query().
		Where(
			externalcohort.ProviderIDEQ(authProviderID),
			externalcohort.CohortKindEQ(cohortKind),
			externalcohort.CohortKeyEQ(cohortKey),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("query observed external cohort %s/%s:%s: %w", authProviderID, cohortKind, cohortKey, err)
	}
	if err == nil {
		update := s.client.ExternalCohort.UpdateOneID(existing.ID).
			SetLastSyncedAt(now)
		if displayName != "" && existing.DisplayName != displayName {
			update = update.SetDisplayName(displayName)
		}
		if _, err := update.Save(ctx); err != nil {
			return fmt.Errorf("update observed external cohort %s/%s:%s: %w", authProviderID, cohortKind, cohortKey, err)
		}
		return nil
	}

	id, uuidErr := uuid.NewV7()
	if uuidErr != nil {
		return fmt.Errorf("generate external cohort id: %w", uuidErr)
	}
	if _, err := s.client.ExternalCohort.Create().
		SetID(id.String()).
		SetProviderID(authProviderID).
		SetCohortKind(cohortKind).
		SetCohortKey(cohortKey).
		SetDisplayName(displayName).
		SetLastSyncedAt(now).
		Save(ctx); err != nil {
		if !ent.IsConstraintError(err) {
			return fmt.Errorf("create observed external cohort %s/%s:%s: %w", authProviderID, cohortKind, cohortKey, err)
		}
		existing, retryErr := s.client.ExternalCohort.Query().
			Where(
				externalcohort.ProviderIDEQ(authProviderID),
				externalcohort.CohortKindEQ(cohortKind),
				externalcohort.CohortKeyEQ(cohortKey),
			).
			Only(ctx)
		if retryErr != nil {
			return fmt.Errorf("requery observed external cohort after constraint %s/%s:%s: %w", authProviderID, cohortKind, cohortKey, retryErr)
		}
		update := s.client.ExternalCohort.UpdateOneID(existing.ID).
			SetLastSyncedAt(now)
		if displayName != "" && existing.DisplayName != displayName {
			update = update.SetDisplayName(displayName)
		}
		if _, retryErr := update.Save(ctx); retryErr != nil {
			return fmt.Errorf("update observed external cohort after constraint %s/%s:%s: %w", authProviderID, cohortKind, cohortKey, retryErr)
		}
	}
	return nil
}

func normalizeExternalAuthResult(result runtimecontract.AuthResult) (runtimecontract.AuthResult, error) {
	result.ExternalID = normalizeCanonicalUserIdentity(result.ExternalID)
	result.Username = normalizeCanonicalUserIdentity(result.Username)
	result.DisplayName = strings.TrimSpace(result.DisplayName)
	result.Email = strings.TrimSpace(strings.ToLower(result.Email))
	result.ProfileAttributes = normalizeDirectoryAttributes(result.ProfileAttributes)
	result.Cohorts = normalizeExternalCohorts(result.Cohorts)
	if result.ExternalID == "" {
		return runtimecontract.AuthResult{}, fmt.Errorf("external_id is required")
	}
	if result.Username == "" {
		return runtimecontract.AuthResult{}, fmt.Errorf("username is required")
	}
	if result.DisplayName == "" {
		result.DisplayName = result.Username
	}
	return result, nil
}

func normalizeCanonicalUserIdentity(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.Contains(trimmed, "@") {
		return strings.ToLower(trimmed)
	}
	return trimmed
}

func normalizeExternalCohorts(cohorts []runtimecontract.ExternalCohort) []runtimecontract.ExternalCohort {
	if len(cohorts) == 0 {
		return nil
	}
	normalized := make([]runtimecontract.ExternalCohort, 0, len(cohorts))
	seen := make(map[string]struct{}, len(cohorts))
	for _, cohort := range cohorts {
		cohort.Kind = strings.TrimSpace(strings.ToLower(cohort.Kind))
		cohort.Key = strings.TrimSpace(cohort.Key)
		cohort.DisplayName = strings.TrimSpace(cohort.DisplayName)
		if cohort.Kind == "" || cohort.Key == "" {
			continue
		}
		dedupeKey := cohort.Kind + "|" + cohort.Key
		if _, exists := seen[dedupeKey]; exists {
			continue
		}
		seen[dedupeKey] = struct{}{}
		normalized = append(normalized, cohort)
	}
	slices.SortFunc(normalized, func(left, right runtimecontract.ExternalCohort) int {
		if cmp := strings.Compare(left.Kind, right.Kind); cmp != 0 {
			return cmp
		}
		return strings.Compare(left.Key, right.Key)
	})
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func externalCohortsToAttributes(cohorts []runtimecontract.ExternalCohort) []map[string]string {
	items := make([]map[string]string, 0, len(cohorts))
	for _, cohort := range cohorts {
		items = append(items, map[string]string{
			"kind":         cohort.Kind,
			"key":          cohort.Key,
			"display_name": cohort.DisplayName,
		})
	}
	return items
}
