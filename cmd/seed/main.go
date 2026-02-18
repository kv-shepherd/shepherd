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

	if err := logger.Init(cfg.Log.Level, cfg.Log.Format); err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer logger.Sync()

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
			ID: "role-bootstrap", Name: "Bootstrap", DisplayName: "Bootstrap Admin",
			Description: "First-run bootstrap role, auto-revoked after setup",
			Permissions: []string{
				// Stage 2.A: bootstrap role keeps explicit super-admin permission only.
				"platform:admin",
			},
		},
		{
			ID: "role-platform-admin", Name: "PlatformAdmin", DisplayName: "Platform Administrator",
			Description: "Full platform management including cluster and security configuration",
			Permissions: []string{
				"platform:admin", // ADR-0019: explicit super-admin permission
			},
		},
		{
			ID: "role-system-admin", Name: "SystemAdmin", DisplayName: "System Administrator",
			Description: "Manages systems and services within assigned scope",
			Permissions: []string{
				"system:read", "system:write", "system:delete",
				"service:read", "service:create", "service:delete",
				"vm:read", "vm:create", "vm:operate", "vm:delete",
				"vnc:access", "rbac:manage",
			},
		},
		{
			ID: "role-approver", Name: "Approver", DisplayName: "Approver",
			Description: "Reviews and approves/rejects VM creation requests",
			Permissions: []string{
				"approval:approve", "approval:view",
				"vm:read", "service:read", "system:read",
			},
		},
		{
			ID: "role-operator", Name: "Operator", DisplayName: "Operator",
			Description: "Day-to-day VM operations within assigned scope",
			Permissions: []string{
				"vm:operate", "vm:create", "vm:read", "vnc:access",
				"system:read", "service:read",
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

// seedBuiltInRoles creates built-in roles with explicit permissions (no wildcards, ADR-0019).
// Uses ON CONFLICT DO NOTHING pattern for idempotency.
func seedBuiltInRoles(ctx context.Context, client *ent.Client) error {
	roles := builtInRoles()
	for _, r := range roles {
		_, err := client.Role.Create().
			SetID(r.ID).
			SetName(r.Name).
			SetDisplayName(r.DisplayName).
			SetDescription(r.Description).
			SetPermissions(r.Permissions).
			SetBuiltIn(true).
			Save(ctx)
		if err != nil {
			// Idempotent: if role already exists, skip (ON CONFLICT DO NOTHING equivalent)
			if ent.IsConstraintError(err) {
				logger.Info("Role already exists, skipping", zap.String("role", r.Name))
				continue
			}
			return fmt.Errorf("create role %s: %w", r.Name, err)
		}
		logger.Info("Seeded built-in role", zap.String("role", r.Name))
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

	user, err := client.User.Create().
		SetID(adminID).
		SetUsername("admin").
		SetEmail("admin@localhost").
		SetDisplayName("Default Administrator").
		SetPasswordHash(hash).
		SetForcePasswordChange(true).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			logger.Info("Default admin already exists, skipping")
			return nil
		}
		return fmt.Errorf("create default admin: %w", err)
	}

	// Assign PlatformAdmin role to default admin
	rbID, _ := uuid.NewV7()
	_, err = client.RoleBinding.Create().
		SetID(rbID.String()).
		SetUserID(user.ID).
		SetRoleID("role-platform-admin").
		SetScopeType("global").
		SetCreatedBy("system-seed").
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			logger.Info("Admin role binding already exists, skipping")
			return nil
		}
		return fmt.Errorf("create admin role binding: %w", err)
	}

	logger.Info("Seeded default admin user",
		zap.String("username", "admin"),
		zap.Bool("force_password_change", true),
	)

	return nil
}
