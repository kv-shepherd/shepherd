package handlers

import (
	"context"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/middleware"
)

type namespaceVisibility struct {
	restricted bool
	envs       []namespaceregistry.Environment
}

// resolveNamespaceVisibility computes namespace visibility for current actor.
//
// Rules:
// - platform:admin => unrestricted
// - no role bindings => restricted with empty env set (no namespace visibility)
// - role bindings with explicit allowed_environments => restricted to that env union
// - role bindings with empty allowed_environments => unrestricted
func (s *Server) resolveNamespaceVisibility(c *gin.Context) (namespaceVisibility, error) {
	if hasPlatformAdmin(c) {
		return namespaceVisibility{restricted: false}, nil
	}

	actor := middleware.GetUserID(c.Request.Context())
	if strings.TrimSpace(actor) == "" {
		return namespaceVisibility{restricted: true, envs: nil}, nil
	}

	bindings, err := s.client.RoleBinding.Query().
		Where(rolebinding.HasUserWith(entuser.IDEQ(actor))).
		All(c.Request.Context())
	if err != nil {
		return namespaceVisibility{}, err
	}
	return namespaceVisibilityFromRoleBindings(bindings), nil
}

func (s *Server) listVisibleNamespaceNames(ctx context.Context, vis namespaceVisibility) ([]string, error) {
	if !vis.restricted {
		return nil, nil
	}
	if len(vis.envs) == 0 {
		return []string{}, nil
	}

	return s.client.NamespaceRegistry.Query().
		Where(namespaceregistry.EnvironmentIn(vis.envs...)).
		Select(namespaceregistry.FieldName).
		Strings(ctx)
}

func (s *Server) isNamespaceVisible(ctx context.Context, namespace string, vis namespaceVisibility) (bool, error) {
	if !vis.restricted {
		return true, nil
	}
	if len(vis.envs) == 0 || strings.TrimSpace(namespace) == "" {
		return false, nil
	}

	return s.client.NamespaceRegistry.Query().
		Where(
			namespaceregistry.NameEQ(strings.TrimSpace(namespace)),
			namespaceregistry.EnvironmentIn(vis.envs...),
		).
		Exist(ctx)
}

func namespaceVisibilityFromRoleBindings(bindings []*ent.RoleBinding) namespaceVisibility {
	if len(bindings) == 0 {
		return namespaceVisibility{restricted: true, envs: nil}
	}

	envSet := map[namespaceregistry.Environment]struct{}{}
	explicitEnvRestriction := false
	for _, rb := range bindings {
		if rb == nil {
			continue
		}
		if len(rb.AllowedEnvironments) == 0 {
			// Empty allowlist means unrestricted by design.
			return namespaceVisibility{restricted: false}
		}
		explicitEnvRestriction = true
		for _, raw := range rb.AllowedEnvironments {
			switch strings.TrimSpace(strings.ToLower(raw)) {
			case string(namespaceregistry.EnvironmentTest):
				envSet[namespaceregistry.EnvironmentTest] = struct{}{}
			case string(namespaceregistry.EnvironmentProd):
				envSet[namespaceregistry.EnvironmentProd] = struct{}{}
			}
		}
	}

	if len(envSet) == 0 {
		if explicitEnvRestriction {
			// Misconfigured explicit restriction must fail closed instead of becoming unrestricted.
			return namespaceVisibility{restricted: true, envs: nil}
		}
		return namespaceVisibility{restricted: false}
	}

	envs := make([]namespaceregistry.Environment, 0, len(envSet))
	for env := range envSet {
		envs = append(envs, env)
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i] < envs[j] })
	return namespaceVisibility{restricted: true, envs: envs}
}
