// Package main provides data seeding for KubeVirt Shepherd.
//
// ADR-0018: Application auto-initializes on first startup.
// This command can be used for explicit seeding outside auto-init.
// master-flow.md Stage 1.5 + Stage 2.A: seed roles + default admin.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/cmd/seed
package main

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/externalcohortmapping"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/infrastructure"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if initErr := logger.Init(cfg.Log.Level, cfg.Log.Format); initErr != nil {
		return fmt.Errorf("init logger: %w", initErr)
	}
	defer func() { _ = logger.Sync() }()

	ctx := context.Background()

	db, err := infrastructure.NewDatabaseClients(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	defer db.Close()

	client := db.EntClient

	logger.Info("Starting data seeding...")

	// Database and River migrations are expected to be executed before seeding.
	// This command only performs idempotent data bootstrap.

	// Seed built-in roles (master-flow.md Stage 2.A, ADR-0015 §22)
	if err := seedBuiltInRoles(ctx, client); err != nil {
		return fmt.Errorf("seed roles: %w", err)
	}

	// Seed default admin user (master-flow.md Stage 1.5)
	if err := seedDefaultAdmin(ctx, client); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	logger.Info("Data seeding completed successfully")
	return nil
}

// builtInRole defines a built-in role for seeding.
type builtInRole struct {
	ID          string
	Name        string
	DisplayName string
	Description string
	Permissions []string
}

func builtInRoles() []builtInRole {
	return []builtInRole{
		{
			ID: "role-platform-admin", Name: "PlatformAdmin", DisplayName: "Platform Administrator",
			Description: "Full platform management including cluster and security configuration",
			Permissions: []string{
				"platform:admin", // ADR-0019: explicit super-admin permission
			},
		},
		{
			ID: "role-approval-admin", Name: "ApprovalAdmin", DisplayName: "Approval Administrator",
			Description: "Reviews and approves built-in request workflows without broader platform administration",
			Permissions: []string{
				"builtin_approval:approve",
				"builtin_approval:view",
				"service:read",
				"system:read",
				"ticket:view",
				"vm:read",
			},
		},
		{
			ID: "role-development-engineer", Name: "DevelopmentEngineer", DisplayName: "Development Engineer",
			Description: "Builds and validates application changes in approved environments without platform or approval administration",
			Permissions: []string{
				"service:create",
				"service:read",
				"system:read",
				"system:write",
				"vm:create",
				"vm:delete",
				"vm:operate",
				"vm:read",
				"vnc:access",
			},
		},
		{
			ID: "role-test-engineer", Name: "TestEngineer", DisplayName: "Test Engineer",
			Description: "Exercises service and VM workflows in approved environments without platform or approval administration",
			Permissions: []string{
				"service:create",
				"service:read",
				"system:read",
				"system:write",
				"vm:create",
				"vm:delete",
				"vm:operate",
				"vm:read",
				"vnc:access",
			},
		},
		{
			ID: "role-system-operator", Name: "SystemOperator", DisplayName: "System Operator",
			Description: "Operates systems and virtual machines in approved environments while still relying on approval workflows for gated production actions",
			Permissions: []string{
				"service:create",
				"service:read",
				"system:read",
				"system:write",
				"vm:create",
				"vm:delete",
				"vm:operate",
				"vm:read",
				"vnc:access",
			},
		},
		{
			ID: "role-viewer", Name: "Viewer", DisplayName: "Viewer",
			Description: "Read-only access to assigned resources",
			Permissions: []string{
				"vm:read", "system:read", "service:read",
			},
		},
	}
}

// seedBuiltInRoles reconciles built-in roles to the current catalog (ADR-0019).
// Built-ins are treated as owned by the application and are updated in place.
func seedBuiltInRoles(ctx context.Context, client *ent.Client) error {
	desiredRoles := builtInRoles()
	desiredByID := make(map[string]builtInRole, len(desiredRoles))
	for _, entry := range desiredRoles {
		desiredByID[entry.ID] = entry
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin role seed transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	existingRoles, err := tx.Role.Query().
		Where(role.BuiltIn(true)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query built-in roles: %w", err)
	}

	for _, entry := range desiredRoles {
		existing, getErr := tx.Role.Get(ctx, entry.ID)
		switch {
		case getErr == nil:
			if _, saveErr := existing.Update().
				SetName(entry.Name).
				SetDisplayName(entry.DisplayName).
				SetDescription(entry.Description).
				SetPermissions(entry.Permissions).
				SetEnabled(true).
				Save(ctx); saveErr != nil {
				return fmt.Errorf("update role %s: %w", entry.Name, saveErr)
			}
			logger.Info("Reconciled built-in role", zap.String("role", entry.Name))
		case ent.IsNotFound(getErr):
			if _, createErr := tx.Role.Create().
				SetID(entry.ID).
				SetName(entry.Name).
				SetDisplayName(entry.DisplayName).
				SetDescription(entry.Description).
				SetPermissions(entry.Permissions).
				SetBuiltIn(true).
				SetEnabled(true).
				Save(ctx); createErr != nil {
				return fmt.Errorf("create role %s: %w", entry.Name, createErr)
			}
			logger.Info("Seeded built-in role", zap.String("role", entry.Name))
		default:
			return fmt.Errorf("load role %s: %w", entry.Name, getErr)
		}
	}

	for _, existing := range existingRoles {
		if _, keep := desiredByID[existing.ID]; keep {
			continue
		}
		if _, deleteErr := tx.ExternalCohortMapping.Delete().
			Where(externalcohortmapping.RoleIDEQ(existing.ID)).
			Exec(ctx); deleteErr != nil {
			return fmt.Errorf("delete external cohort mappings for obsolete built-in role %s: %w", existing.ID, deleteErr)
		}
		if _, deleteErr := tx.ExternalCohortGrant.Delete().
			Where(
				externalcohortgrant.HasRoleBindingWith(
					rolebinding.HasRoleWith(role.IDEQ(existing.ID)),
				),
			).
			Exec(ctx); deleteErr != nil {
			return fmt.Errorf("delete external cohort grants for obsolete built-in role %s: %w", existing.ID, deleteErr)
		}
		if _, deleteErr := tx.RoleBinding.Delete().
			Where(rolebinding.HasRoleWith(role.IDEQ(existing.ID))).
			Exec(ctx); deleteErr != nil {
			return fmt.Errorf("delete bindings for obsolete built-in role %s: %w", existing.ID, deleteErr)
		}
		if deleteErr := tx.Role.DeleteOneID(existing.ID).Exec(ctx); deleteErr != nil {
			return fmt.Errorf("delete obsolete built-in role %s: %w", existing.ID, deleteErr)
		}
		logger.Info("Removed obsolete built-in role", zap.String("role_id", existing.ID), zap.String("role", existing.Name))
	}

	liveRoleIDs, err := tx.Role.Query().Select(role.FieldID).Strings(ctx)
	if err != nil {
		return fmt.Errorf("query live role ids after reconciliation: %w", err)
	}
	if len(liveRoleIDs) == 0 {
		return fmt.Errorf("role reconciliation produced an empty role catalog")
	}
	if removed, err := tx.ExternalCohortMapping.Delete().
		Where(externalcohortmapping.RoleIDNotIn(liveRoleIDs...)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete stale external cohort mappings with missing roles: %w", err)
	} else if removed > 0 {
		logger.Info("Removed stale external cohort mappings with missing roles", zap.Int("count", removed))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit role seed transaction: %w", err)
	}
	return nil
}

// seedDefaultAdmin creates the default admin user (admin/admin, force_password_change=true).
// master-flow.md Stage 1.5: Default admin with forced password change.
func seedDefaultAdmin(ctx context.Context, client *ent.Client) error {
	adminID := "user-default-admin"
	// bcrypt hash for default password (force_password_change=true ensures change on first login)
	hashBytes, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash default admin password: %w", err)
	}
	hash := string(hashBytes)

	created := false
	user, err := client.User.Create().
		SetID(adminID).
		SetUsername("admin").
		SetEmail("admin@localhost").
		SetDisplayName("Default Administrator").
		SetPasswordHash(hash).
		SetForcePasswordChange(true).
		Save(ctx)
	switch {
	case err == nil:
		created = true
	case ent.IsConstraintError(err):
		user, err = client.User.Query().
			Where(entuser.IDEQ(adminID)).
			Only(ctx)
		if ent.IsNotFound(err) {
			user, err = client.User.Query().
				Where(entuser.UsernameEQ("admin")).
				Only(ctx)
		}
		if err != nil {
			return fmt.Errorf("load default admin after constraint: %w", err)
		}
	default:
		return fmt.Errorf("create default admin: %w", err)
	}

	bindingExists, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(entuser.IDEQ(user.ID)),
			rolebinding.HasRoleWith(role.IDEQ("role-platform-admin")),
			rolebinding.ScopeTypeEQ("global"),
		).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check admin role binding: %w", err)
	}
	if !bindingExists {
		rbID, _ := uuid.NewV7()
		if _, err := client.RoleBinding.Create().
			SetID(rbID.String()).
			SetUserID(user.ID).
			SetRoleID("role-platform-admin").
			SetScopeType("global").
			SetCreatedBy("system-seed").
			Save(ctx); err != nil {
			return fmt.Errorf("create admin role binding: %w", err)
		}
	}

	logger.Info("Ensured default admin user",
		zap.String("username", "admin"),
		zap.Bool("created", created),
		zap.Bool("force_password_change", true),
	)

	return nil
}
