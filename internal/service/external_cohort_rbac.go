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
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
)

const externalCohortRoleBindingActor = "system:external-cohort-mapper"

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
) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("external auth service is not initialized")
	}

	desiredBindings, err := s.buildDesiredExternalCohortBindings(ctx, providerID, cohorts)
	if err != nil {
		return err
	}

	existingGrants, err := s.client.ExternalCohortGrant.Query().
		Where(
			externalcohortgrant.UserIDEQ(userID),
			externalcohortgrant.ProviderIDEQ(providerID),
		).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query external cohort grants: %w", err)
	}

	desiredByKey := make(map[string]desiredExternalCohortBinding, len(desiredBindings))
	for _, desired := range desiredBindings {
		desiredByKey[desired.BindingKey] = desired
	}

	now := time.Now().UTC()
	for _, grant := range existingGrants {
		desired, keep := desiredByKey[grant.BindingKey]
		if !keep {
			if err := s.deleteExternalCohortGrant(ctx, grant); err != nil {
				return err
			}
			continue
		}
		roleBindingID, err := s.ensureManagedRoleBinding(ctx, userID, desired, grant.RoleBindingID)
		if err != nil {
			return err
		}
		update := s.client.ExternalCohortGrant.UpdateOneID(grant.ID).SetLastAppliedAt(now)
		if roleBindingID != grant.RoleBindingID {
			update = update.SetRoleBindingID(roleBindingID)
		}
		if !slices.Equal(grant.SourceMappingIds, desired.SourceMappingIDs) {
			update = update.SetSourceMappingIds(cloneStringSlice(desired.SourceMappingIDs))
		}
		if _, err := update.Save(ctx); err != nil {
			return fmt.Errorf("update external cohort grant %s: %w", grant.ID, err)
		}
		delete(desiredByKey, grant.BindingKey)
	}

	for _, desired := range desiredByKey {
		roleBindingID, err := s.ensureManagedRoleBinding(ctx, userID, desired, "")
		if err != nil {
			return err
		}
		id, uuidErr := uuid.NewV7()
		if uuidErr != nil {
			return fmt.Errorf("generate external cohort grant id: %w", uuidErr)
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
			return fmt.Errorf("create external cohort grant: %w", err)
		}
	}

	return nil
}

func (s *ExternalAuthService) buildDesiredExternalCohortBindings(
	ctx context.Context,
	providerID string,
	cohorts []runtimecontract.ExternalCohort,
) ([]desiredExternalCohortBinding, error) {
	mappings, err := s.client.ExternalCohortMapping.Query().
		Where(externalcohortmapping.ProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query external cohort mappings: %w", err)
	}
	if len(mappings) == 0 || len(cohorts) == 0 {
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
		cohortRef := strings.TrimSpace(strings.ToLower(mapping.CohortKind)) + "|" + strings.TrimSpace(mapping.CohortKey)
		if _, matched := cohortSet[cohortRef]; !matched {
			continue
		}

		allowedEnvs := cloneStringSlice(mapping.AllowedEnvironments)
		sort.Strings(allowedEnvs)
		bindingKey := externalCohortBindingKey(mapping.RoleID, mapping.ScopeType, mapping.ScopeID, allowedEnvs)
		entry, exists := grouped[bindingKey]
		if !exists {
			entry = &desiredExternalCohortBinding{
				BindingKey:          bindingKey,
				RoleID:              mapping.RoleID,
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

func (s *ExternalAuthService) ensureManagedRoleBinding(
	ctx context.Context,
	userID string,
	desired desiredExternalCohortBinding,
	existingRoleBindingID string,
) (string, error) {
	if existingRoleBindingID != "" {
		_, err := s.client.RoleBinding.Query().
			Where(rolebinding.IDEQ(existingRoleBindingID), rolebinding.HasUserWith(entuser.IDEQ(userID))).
			Only(ctx)
		if err == nil {
			return existingRoleBindingID, nil
		} else if !ent.IsNotFound(err) {
			return "", fmt.Errorf("query managed role binding %s: %w", existingRoleBindingID, err)
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate role binding id: %w", err)
	}
	create := s.client.RoleBinding.Create().
		SetID(id.String()).
		SetUserID(userID).
		SetRoleID(desired.RoleID).
		SetCreatedBy(externalCohortRoleBindingActor)
	if desired.ScopeType != "" {
		create = create.SetScopeType(desired.ScopeType)
	} else {
		create = create.SetScopeType("global")
	}
	if desired.ScopeID != "" {
		create = create.SetScopeID(desired.ScopeID)
	}
	if len(desired.AllowedEnvironments) > 0 {
		create = create.SetAllowedEnvironments(cloneStringSlice(desired.AllowedEnvironments))
	}
	binding, err := create.Save(ctx)
	if err != nil {
		return "", fmt.Errorf("create managed role binding: %w", err)
	}
	return binding.ID, nil
}

func (s *ExternalAuthService) deleteExternalCohortGrant(ctx context.Context, grant *ent.ExternalCohortGrant) error {
	if grant == nil {
		return nil
	}
	if err := s.client.ExternalCohortGrant.DeleteOneID(grant.ID).Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("delete external cohort grant %s: %w", grant.ID, err)
	}
	if grant.RoleBindingID != "" {
		if err := s.client.RoleBinding.DeleteOneID(grant.RoleBindingID).Exec(ctx); err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("delete managed role binding %s: %w", grant.RoleBindingID, err)
		}
	}
	return nil
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
