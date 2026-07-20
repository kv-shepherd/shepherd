package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/service"
)

var (
	errRoleAssignmentNotFound = errors.New("role assignment target not found")
	errRoleAssignmentDisabled = errors.New("role assignment target is disabled")
)

func lockRoleRow(ctx context.Context, tx *ent.Tx, roleID string) error {
	return service.LockRoleAssignment(ctx, tx, roleID)
}

func loadEnabledRoleForAssignment(ctx context.Context, client *ent.Client, roleID string) (*ent.Role, error) {
	if client == nil {
		return nil, fmt.Errorf("ent client is required")
	}
	roleEnt, err := client.Role.Get(ctx, strings.TrimSpace(roleID))
	if ent.IsNotFound(err) {
		return nil, errRoleAssignmentNotFound
	}
	if err != nil {
		return nil, err
	}
	if !roleEnt.Enabled {
		return nil, errRoleAssignmentDisabled
	}
	return roleEnt, nil
}
