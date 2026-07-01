package service

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/externalcohortmapping"
	entrole "kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
)

const (
	externalCohortGlobalScopeType  = "global"
	externalCohortRoleBindingActor = "system:external-cohort-mapper"
)

type desiredExternalCohortBinding struct {
	BindingKey          string
	RoleID              string
	ScopeType           string
	ScopeID             string
	AllowedEnvironments []string
	SourceMappingIDs    []string
}

func (s *ExternalAuthService) reconcileExternalCohortRBAC(
	ctx context.Context,
	userID, providerID string,
	cohorts []runtimecontract.ExternalCohort,
) (bool, error) {
	if s == nil || s.client == nil {
		return false, fmt.Errorf("external auth service is not initialized")
	}

	desiredBindings, err := s.buildDesiredExternalCohortBindings(ctx, providerID, cohorts)
	if err != nil {
		return false, err
	}

	existingGrants, err := s.client.ExternalCohortGrant.Query().
		Where(
			externalcohortgrant.UserIDEQ(userID),
			externalcohortgrant.ProviderIDEQ(providerID),
		).
		All(ctx)
	if err != nil {
		return false, fmt.Errorf("query external cohort grants: %w", err)
	}

	rbacChanged := false
	desiredByKey := make(map[string]desiredExternalCohortBinding, len(desiredBindings))
	for _, desired := range desiredBindings {
		desiredByKey[desired.BindingKey] = desired
	}

	now := time.Now().UTC()
	for _, grant := range existingGrants {
		desired, keep := desiredByKey[grant.BindingKey]
		if !keep {
			if err := s.deleteExternalCohortGrant(ctx, s.client, grant); err != nil {
				return false, err
			}
			rbacChanged = true
			continue
		}
		roleBindingID, roleBindingChanged, err := s.ensureManagedRoleBinding(ctx, s.client, userID, desired, grant.RoleBindingID)
		if err != nil {
			return false, err
		}
		if roleBindingChanged {
			rbacChanged = true
		}
		update := s.client.ExternalCohortGrant.UpdateOneID(grant.ID).SetLastAppliedAt(now)
		if roleBindingID != grant.RoleBindingID {
			update = update.SetRoleBindingID(roleBindingID)
		}
		if !slices.Equal(grant.SourceMappingIds, desired.SourceMappingIDs) {
			update = update.SetSourceMappingIds(cloneStringSlice(desired.SourceMappingIDs))
		}
		if _, err := update.Save(ctx); err != nil {
			return false, fmt.Errorf("update external cohort grant %s: %w", grant.ID, err)
		}
		delete(desiredByKey, grant.BindingKey)
	}

	for _, desired := range desiredByKey {
		id, uuidErr := uuid.NewV7()
		if uuidErr != nil {
			return false, fmt.Errorf("generate external cohort grant id: %w", uuidErr)
		}
		roleBindingID, _, err := s.ensureManagedRoleBinding(ctx, s.client, userID, desired, "")
		if err != nil {
			return false, err
		}
		if _, err := s.client.ExternalCohortGrant.Create().
			SetID(id.String()).
			SetUserID(userID).
			SetProviderID(providerID).
			SetBindingKey(desired.BindingKey).
			SetRoleBindingID(roleBindingID).
			SetSourceMappingIds(cloneStringSlice(desired.SourceMappingIDs)).
			SetLastAppliedAt(now).
			Save(ctx); err != nil {
			return false, fmt.Errorf("create external cohort grant: %w", err)
		}
		rbacChanged = true
	}

	return rbacChanged, nil
}

func (s *ExternalAuthService) buildDesiredExternalCohortBindings(
	ctx context.Context,
	providerID string,
	cohorts []runtimecontract.ExternalCohort,
) ([]desiredExternalCohortBinding, error) {
	if len(cohorts) == 0 {
		return nil, nil
	}

	mappings, err := s.client.ExternalCohortMapping.Query().
		Where(externalcohortmapping.ProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query external cohort mappings: %w", err)
	}
	if len(mappings) == 0 {
		return nil, nil
	}

	enabledRoleIDs, err := s.enabledExternalCohortMappingRoleIDSet(ctx, mappings)
	if err != nil {
		return nil, err
	}
	if len(enabledRoleIDs) == 0 {
		return nil, nil
	}

	cohortSet := make(map[string]struct{}, len(cohorts))
	for _, cohort := range cohorts {
		key := strings.TrimSpace(strings.ToLower(cohort.Kind)) + "|" + strings.TrimSpace(cohort.Key)
		if key != "|" {
			cohortSet[key] = struct{}{}
		}
	}

	grouped := make(map[string]*desiredExternalCohortBinding)
	for _, mapping := range mappings {
		roleID := strings.TrimSpace(mapping.RoleID)
		if _, ok := enabledRoleIDs[roleID]; !ok {
			continue
		}
		cohortRef := strings.TrimSpace(strings.ToLower(mapping.CohortKind)) + "|" + strings.TrimSpace(mapping.CohortKey)
		if _, matched := cohortSet[cohortRef]; !matched {
			continue
		}

		allowedEnvs := cloneStringSlice(mapping.AllowedEnvironments)
		sort.Strings(allowedEnvs)
		bindingKey := externalCohortBindingKey(roleID, mapping.ScopeType, mapping.ScopeID, allowedEnvs)
		entry, exists := grouped[bindingKey]
		if !exists {
			entry = &desiredExternalCohortBinding{
				BindingKey:          bindingKey,
				RoleID:              roleID,
				ScopeType:           strings.TrimSpace(mapping.ScopeType),
				ScopeID:             strings.TrimSpace(mapping.ScopeID),
				AllowedEnvironments: allowedEnvs,
			}
			grouped[bindingKey] = entry
		}
		entry.SourceMappingIDs = append(entry.SourceMappingIDs, mapping.ID)
	}

	out := make([]desiredExternalCohortBinding, 0, len(grouped))
	for _, entry := range grouped {
		sort.Strings(entry.SourceMappingIDs)
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BindingKey < out[j].BindingKey })
	return out, nil
}

func (s *ExternalAuthService) enabledExternalCohortMappingRoleIDSet(
	ctx context.Context,
	mappings []*ent.ExternalCohortMapping,
) (map[string]struct{}, error) {
	roleIDs := make([]string, 0, len(mappings))
	seenRoleIDs := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		if mapping == nil {
			continue
		}
		roleID := strings.TrimSpace(mapping.RoleID)
		if roleID == "" {
			continue
		}
		if _, ok := seenRoleIDs[roleID]; ok {
			continue
		}
		seenRoleIDs[roleID] = struct{}{}
		roleIDs = append(roleIDs, roleID)
	}
	if len(roleIDs) == 0 {
		return nil, nil
	}

	enabledRoleIDs, err := s.client.Role.Query().
		Where(
			entrole.IDIn(roleIDs...),
			entrole.EnabledEQ(true),
		).
		IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("query enabled external cohort mapping roles: %w", err)
	}

	out := make(map[string]struct{}, len(enabledRoleIDs))
	for _, roleID := range enabledRoleIDs {
		out[roleID] = struct{}{}
	}
	return out, nil
}

func (s *ExternalAuthService) ensureManagedRoleBinding(
	ctx context.Context,
	client *ent.Client,
	userID string,
	desired desiredExternalCohortBinding,
	existingRoleBindingID string,
) (roleBindingID string, changed bool, err error) {
	if existingRoleBindingID != "" {
		existing, queryErr := client.RoleBinding.Query().
			Where(
				rolebinding.IDEQ(existingRoleBindingID),
				rolebinding.HasUserWith(entuser.IDEQ(userID)),
			).
			WithRole().
			Only(ctx)
		if queryErr == nil {
			if existing.CreatedBy != externalCohortRoleBindingActor {
				roleBindingID, err = s.createManagedRoleBinding(ctx, client, userID, desired)
				return roleBindingID, true, err
			}
			existingRole, roleErr := existing.Edges.RoleOrErr()
			if roleErr != nil {
				return "", false, fmt.Errorf("query managed role binding %s role: %w", existingRoleBindingID, roleErr)
			}
			if externalCohortRoleBindingMatchesDesired(existing, existingRole.ID, desired) {
				return existingRoleBindingID, false, nil
			}
			roleBindingID, err = s.updateManagedRoleBinding(ctx, client, userID, existingRoleBindingID, desired)
			return roleBindingID, true, err
		} else if !ent.IsNotFound(queryErr) {
			return "", false, fmt.Errorf("query managed role binding %s: %w", existingRoleBindingID, queryErr)
		}
	}

	roleBindingID, err = s.createManagedRoleBinding(ctx, client, userID, desired)
	return roleBindingID, true, err
}

func (s *ExternalAuthService) createManagedRoleBinding(
	ctx context.Context,
	client *ent.Client,
	userID string,
	desired desiredExternalCohortBinding,
) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate role binding id: %w", err)
	}
	create := client.RoleBinding.Create().
		SetID(id.String()).
		SetUserID(userID).
		SetRoleID(desired.RoleID).
		SetScopeType(externalCohortRoleBindingScopeType(desired.ScopeType)).
		SetCreatedBy(externalCohortRoleBindingActor)
	if scopeID := strings.TrimSpace(desired.ScopeID); scopeID != "" {
		create = create.SetScopeID(scopeID)
	}
	if allowedEnvironments := normalizedExternalCohortRoleBindingEnvironments(desired.AllowedEnvironments); len(allowedEnvironments) > 0 {
		create = create.SetAllowedEnvironments(allowedEnvironments)
	}
	binding, err := create.Save(ctx)
	if err != nil {
		return "", fmt.Errorf("create managed role binding: %w", err)
	}
	return binding.ID, nil
}

func (s *ExternalAuthService) updateManagedRoleBinding(
	ctx context.Context,
	client *ent.Client,
	userID, roleBindingID string,
	desired desiredExternalCohortBinding,
) (string, error) {
	update := client.RoleBinding.UpdateOneID(roleBindingID).
		Where(
			rolebinding.CreatedByEQ(externalCohortRoleBindingActor),
			rolebinding.HasUserWith(entuser.IDEQ(userID)),
		).
		SetRoleID(desired.RoleID).
		SetScopeType(externalCohortRoleBindingScopeType(desired.ScopeType))
	if scopeID := strings.TrimSpace(desired.ScopeID); scopeID != "" {
		update = update.SetScopeID(scopeID)
	} else {
		update = update.ClearScopeID()
	}
	if allowedEnvironments := normalizedExternalCohortRoleBindingEnvironments(desired.AllowedEnvironments); len(allowedEnvironments) > 0 {
		update = update.SetAllowedEnvironments(allowedEnvironments)
	} else {
		update = update.ClearAllowedEnvironments()
	}

	binding, err := update.Save(ctx)
	if err == nil {
		return binding.ID, nil
	}
	if ent.IsNotFound(err) {
		return s.createManagedRoleBinding(ctx, client, userID, desired)
	}
	return "", fmt.Errorf("update managed role binding %s: %w", roleBindingID, err)
}

func (s *ExternalAuthService) deleteExternalCohortGrant(ctx context.Context, client *ent.Client, grant *ent.ExternalCohortGrant) error {
	if grant == nil {
		return nil
	}
	if err := client.ExternalCohortGrant.DeleteOneID(grant.ID).Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("delete external cohort grant %s: %w", grant.ID, err)
	}
	if grant.RoleBindingID != "" {
		if err := client.RoleBinding.DeleteOneID(grant.RoleBindingID).
			Where(
				rolebinding.CreatedByEQ(externalCohortRoleBindingActor),
				rolebinding.HasUserWith(entuser.IDEQ(grant.UserID)),
			).
			Exec(ctx); err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("delete managed role binding %s: %w", grant.RoleBindingID, err)
		}
	}
	return nil
}

func (s *ExternalAuthService) deleteExternalCohortGrantsForUser(
	ctx context.Context,
	userID string,
	retainedProviderID string,
) (bool, error) {
	if s == nil || s.client == nil {
		return false, fmt.Errorf("external auth service is not initialized")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, fmt.Errorf("user id is required")
	}
	retainedProviderID = strings.TrimSpace(retainedProviderID)

	query := s.client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.UserIDEQ(userID))
	if retainedProviderID != "" {
		query = query.Where(externalcohortgrant.ProviderIDNEQ(retainedProviderID))
	}
	grants, err := query.All(ctx)
	if err != nil {
		return false, fmt.Errorf("query stale external cohort grants: %w", err)
	}
	for _, grant := range grants {
		if err := s.deleteExternalCohortGrant(ctx, s.client, grant); err != nil {
			return false, err
		}
	}
	return len(grants) > 0, nil
}

func externalCohortRoleBindingMatchesDesired(binding *ent.RoleBinding, roleID string, desired desiredExternalCohortBinding) bool {
	if binding == nil {
		return false
	}
	if roleID != desired.RoleID {
		return false
	}
	if binding.ScopeType != externalCohortRoleBindingScopeType(desired.ScopeType) {
		return false
	}
	if strings.TrimSpace(binding.ScopeID) != strings.TrimSpace(desired.ScopeID) {
		return false
	}
	return slices.Equal(
		normalizedExternalCohortRoleBindingEnvironments(binding.AllowedEnvironments),
		normalizedExternalCohortRoleBindingEnvironments(desired.AllowedEnvironments),
	)
}

func externalCohortRoleBindingScopeType(scopeType string) string {
	scopeType = strings.TrimSpace(scopeType)
	if scopeType == "" {
		return externalCohortGlobalScopeType
	}
	return scopeType
}

func normalizedExternalCohortRoleBindingEnvironments(environments []string) []string {
	normalized := make([]string, 0, len(environments))
	for _, env := range environments {
		env = strings.TrimSpace(env)
		if env == "" {
			continue
		}
		normalized = append(normalized, env)
	}
	sort.Strings(normalized)
	return normalized
}

func externalCohortBindingKey(roleID, scopeType, scopeID string, allowedEnvironments []string) string {
	normalizedEnvs := cloneStringSlice(allowedEnvironments)
	sort.Strings(normalizedEnvs)
	return strings.Join([]string{
		strings.TrimSpace(roleID),
		strings.TrimSpace(scopeType),
		strings.TrimSpace(scopeID),
		strings.Join(normalizedEnvs, ","),
	}, "|")
}
