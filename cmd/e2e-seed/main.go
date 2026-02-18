// Package main seeds deterministic fixtures for live end-to-end tests.
//
// This command is test-environment only and is intentionally idempotent.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entinstancesize "kv-shepherd.io/shepherd/ent/instancesize"
	entnamespaceregistry "kv-shepherd.io/shepherd/ent/namespaceregistry"
	entrole "kv-shepherd.io/shepherd/ent/role"
	entrolebinding "kv-shepherd.io/shepherd/ent/rolebinding"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entsystem "kv-shepherd.io/shepherd/ent/system"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entuser "kv-shepherd.io/shepherd/ent/user"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/infrastructure"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	defaultAdminUsername = "e2e-admin"
	defaultAdminPassword = "e2e-admin-123"
	defaultAdminEmail    = "e2e-admin@localhost"

	defaultNamespaceName = "e2e-test"
	defaultClusterName   = "e2e-cluster"
	defaultSystemName    = "e2e-system"
	defaultServiceName   = "e2e-service"
	defaultTemplateName  = "e2e-template"
	defaultSizeName      = "e2e-small"

	defaultRunningVMID = "vm-e2e-running"
	defaultStoppedVMID = "vm-e2e-stopped"
)

type fixtureConfig struct {
	AdminUsername string
	AdminPassword string
	AdminEmail    string

	NamespaceName string
	ClusterName   string
	SystemName    string
	ServiceName   string
	TemplateName  string
	SizeName      string

	RunningVMID string
	StoppedVMID string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e-seed error: %v\n", err)
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

	fx := loadFixtureConfig()
	client := db.EntClient

	adminID, err := ensureAdminUser(ctx, client, fx)
	if err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}
	if err := ensureAdminRoleBinding(ctx, client, adminID); err != nil {
		return fmt.Errorf("ensure admin role binding: %w", err)
	}

	if err := ensureNamespaceRegistry(ctx, client, fx); err != nil {
		return fmt.Errorf("ensure namespace: %w", err)
	}
	clusterID, err := ensureCluster(ctx, client, fx)
	if err != nil {
		return fmt.Errorf("ensure cluster: %w", err)
	}
	systemID, err := ensureSystem(ctx, client, fx)
	if err != nil {
		return fmt.Errorf("ensure system: %w", err)
	}
	serviceID, err := ensureService(ctx, client, fx, systemID)
	if err != nil {
		return fmt.Errorf("ensure service: %w", err)
	}
	if err := ensureTemplate(ctx, client, fx); err != nil {
		return fmt.Errorf("ensure template: %w", err)
	}
	if err := ensureInstanceSize(ctx, client, fx); err != nil {
		return fmt.Errorf("ensure instance size: %w", err)
	}
	if err := ensureVM(ctx, client, fx.RunningVMID, "vm-live", "01", entvm.StatusRUNNING, fx.NamespaceName, clusterID, serviceID, fx.AdminUsername); err != nil {
		return fmt.Errorf("ensure running vm: %w", err)
	}
	if err := ensureVM(ctx, client, fx.StoppedVMID, "vm-stopped", "02", entvm.StatusSTOPPED, fx.NamespaceName, clusterID, serviceID, fx.AdminUsername); err != nil {
		return fmt.Errorf("ensure stopped vm: %w", err)
	}

	fmt.Printf("e2e fixtures ready (user=%s namespace=%s system=%s service=%s)\n",
		fx.AdminUsername, fx.NamespaceName, fx.SystemName, fx.ServiceName,
	)
	return nil
}

func loadFixtureConfig() fixtureConfig {
	return fixtureConfig{
		AdminUsername: envOrDefault("E2E_ADMIN_USERNAME", defaultAdminUsername),
		AdminPassword: envOrDefault("E2E_ADMIN_PASSWORD", defaultAdminPassword),
		AdminEmail:    envOrDefault("E2E_ADMIN_EMAIL", defaultAdminEmail),
		NamespaceName: envOrDefault("E2E_NAMESPACE", defaultNamespaceName),
		ClusterName:   envOrDefault("E2E_CLUSTER", defaultClusterName),
		SystemName:    envOrDefault("E2E_SYSTEM", defaultSystemName),
		ServiceName:   envOrDefault("E2E_SERVICE", defaultServiceName),
		TemplateName:  envOrDefault("E2E_TEMPLATE", defaultTemplateName),
		SizeName:      envOrDefault("E2E_SIZE", defaultSizeName),
		RunningVMID:   envOrDefault("E2E_VM_RUNNING_ID", defaultRunningVMID),
		StoppedVMID:   envOrDefault("E2E_VM_STOPPED_ID", defaultStoppedVMID),
	}
}

func envOrDefault(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func ensureAdminUser(ctx context.Context, client *ent.Client, fx fixtureConfig) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(fx.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	user, err := client.User.Query().Where(entuser.UsernameEQ(fx.AdminUsername)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return "", err
		}
		id, _ := uuid.NewV7()
		created, createErr := client.User.Create().
			SetID(id.String()).
			SetUsername(fx.AdminUsername).
			SetEmail(fx.AdminEmail).
			SetDisplayName("E2E Administrator").
			SetPasswordHash(string(hash)).
			SetForcePasswordChange(false).
			SetEnabled(true).
			Save(ctx)
		if createErr != nil {
			return "", createErr
		}
		return created.ID, nil
	}

	updated, err := client.User.UpdateOneID(user.ID).
		SetEmail(fx.AdminEmail).
		SetDisplayName("E2E Administrator").
		SetPasswordHash(string(hash)).
		SetForcePasswordChange(false).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		return "", err
	}
	return updated.ID, nil
}

func ensureAdminRoleBinding(ctx context.Context, client *ent.Client, userID string) error {
	roleObj, err := client.Role.Query().Where(entrole.NameEQ("PlatformAdmin")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("PlatformAdmin role not found, run cmd/seed first")
		}
		return err
	}

	exists, err := client.RoleBinding.Query().
		Where(
			entrolebinding.HasUserWith(entuser.IDEQ(userID)),
			entrolebinding.HasRoleWith(entrole.IDEQ(roleObj.ID)),
		).
		Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	rbID, _ := uuid.NewV7()
	_, err = client.RoleBinding.Create().
		SetID(rbID.String()).
		SetUserID(userID).
		SetRoleID(roleObj.ID).
		SetScopeType("global").
		SetCreatedBy("e2e-seed").
		Save(ctx)
	return err
}

func ensureNamespaceRegistry(ctx context.Context, client *ent.Client, fx fixtureConfig) error {
	ns, err := client.NamespaceRegistry.Query().
		Where(entnamespaceregistry.NameEQ(fx.NamespaceName)).
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return err
		}
		id, _ := uuid.NewV7()
		_, createErr := client.NamespaceRegistry.Create().
			SetID(id.String()).
			SetName(fx.NamespaceName).
			SetEnvironment(entnamespaceregistry.EnvironmentTest).
			SetDescription("e2e namespace").
			SetCreatedBy("e2e-seed").
			SetEnabled(true).
			Save(ctx)
		return createErr
	}

	_, err = client.NamespaceRegistry.UpdateOneID(ns.ID).
		SetEnvironment(entnamespaceregistry.EnvironmentTest).
		SetDescription("e2e namespace").
		SetEnabled(true).
		Save(ctx)
	return err
}

func ensureCluster(ctx context.Context, client *ent.Client, fx fixtureConfig) (string, error) {
	kubeconfig := []byte("apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n")

	obj, err := client.Cluster.Query().Where(entcluster.NameEQ(fx.ClusterName)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return "", err
		}
		id, _ := uuid.NewV7()
		created, createErr := client.Cluster.Create().
			SetID(id.String()).
			SetName(fx.ClusterName).
			SetDisplayName("E2E Cluster").
			SetAPIServerURL("https://e2e.invalid").
			SetEncryptedKubeconfig(kubeconfig).
			SetStatus(entcluster.StatusHEALTHY).
			SetEnvironment(entcluster.EnvironmentTest).
			SetEnabled(true).
			SetCreatedBy("e2e-seed").
			Save(ctx)
		if createErr != nil {
			return "", createErr
		}
		return created.ID, nil
	}

	updated, err := client.Cluster.UpdateOneID(obj.ID).
		SetDisplayName("E2E Cluster").
		SetAPIServerURL("https://e2e.invalid").
		SetEncryptedKubeconfig(kubeconfig).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnvironment(entcluster.EnvironmentTest).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		return "", err
	}
	return updated.ID, nil
}

func ensureSystem(ctx context.Context, client *ent.Client, fx fixtureConfig) (string, error) {
	obj, err := client.System.Query().Where(entsystem.NameEQ(fx.SystemName)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return "", err
		}
		id, _ := uuid.NewV7()
		created, createErr := client.System.Create().
			SetID(id.String()).
			SetName(fx.SystemName).
			SetDescription("e2e system").
			SetCreatedBy("e2e-seed").
			SetTenantID("default").
			Save(ctx)
		if createErr != nil {
			return "", createErr
		}
		return created.ID, nil
	}

	updated, err := client.System.UpdateOneID(obj.ID).
		SetDescription("e2e system").
		Save(ctx)
	if err != nil {
		return "", err
	}
	return updated.ID, nil
}

func ensureService(ctx context.Context, client *ent.Client, fx fixtureConfig, systemID string) (string, error) {
	obj, err := client.Service.Query().
		Where(
			entservice.NameEQ(fx.ServiceName),
			entservice.HasSystemWith(entsystem.IDEQ(systemID)),
		).
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return "", err
		}
		id, _ := uuid.NewV7()
		created, createErr := client.Service.Create().
			SetID(id.String()).
			SetName(fx.ServiceName).
			SetDescription("e2e service").
			SetSystemID(systemID).
			SetNextInstanceIndex(3).
			Save(ctx)
		if createErr != nil {
			return "", createErr
		}
		return created.ID, nil
	}

	updated, err := client.Service.UpdateOneID(obj.ID).
		SetDescription("e2e service").
		SetNextInstanceIndex(3).
		Save(ctx)
	if err != nil {
		return "", err
	}
	return updated.ID, nil
}

func ensureTemplate(ctx context.Context, client *ent.Client, fx fixtureConfig) error {
	obj, err := client.Template.Query().
		Where(
			enttemplate.NameEQ(fx.TemplateName),
			enttemplate.VersionEQ(1),
		).
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return err
		}
		id, _ := uuid.NewV7()
		_, createErr := client.Template.Create().
			SetID(id.String()).
			SetName(fx.TemplateName).
			SetDisplayName("E2E Template").
			SetDescription("e2e template").
			SetVersion(1).
			SetEnabled(true).
			SetCreatedBy("e2e-seed").
			Save(ctx)
		return createErr
	}

	_, err = client.Template.UpdateOneID(obj.ID).
		SetDisplayName("E2E Template").
		SetDescription("e2e template").
		SetEnabled(true).
		Save(ctx)
	return err
}

func ensureInstanceSize(ctx context.Context, client *ent.Client, fx fixtureConfig) error {
	obj, err := client.InstanceSize.Query().
		Where(entinstancesize.NameEQ(fx.SizeName)).
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return err
		}
		id, _ := uuid.NewV7()
		_, createErr := client.InstanceSize.Create().
			SetID(id.String()).
			SetName(fx.SizeName).
			SetDisplayName("E2E Small").
			SetDescription("e2e size").
			SetCPUCores(2).
			SetMemoryMB(4096).
			SetDiskGB(40).
			SetEnabled(true).
			SetCreatedBy("e2e-seed").
			Save(ctx)
		return createErr
	}

	_, err = client.InstanceSize.UpdateOneID(obj.ID).
		SetDisplayName("E2E Small").
		SetDescription("e2e size").
		SetCPUCores(2).
		SetMemoryMB(4096).
		SetDiskGB(40).
		SetEnabled(true).
		Save(ctx)
	return err
}

func ensureVM(
	ctx context.Context,
	client *ent.Client,
	id string,
	name string,
	instance string,
	status entvm.Status,
	namespace string,
	clusterID string,
	serviceID string,
	createdBy string,
) error {
	obj, err := client.VM.Get(ctx, id)
	if err != nil {
		if !ent.IsNotFound(err) {
			return err
		}
		_, createErr := client.VM.Create().
			SetID(id).
			SetName(name).
			SetInstance(instance).
			SetNamespace(namespace).
			SetClusterID(clusterID).
			SetStatus(status).
			SetHostname(fmt.Sprintf("%s.%s.local", name, namespace)).
			SetCreatedBy(createdBy).
			SetServiceID(serviceID).
			Save(ctx)
		return createErr
	}

	_, err = client.VM.UpdateOneID(obj.ID).
		SetClusterID(clusterID).
		SetNamespace(namespace).
		SetStatus(status).
		SetHostname(fmt.Sprintf("%s.%s.local", name, namespace)).
		SetCreatedBy(createdBy).
		SetServiceID(serviceID).
		Save(ctx)
	return err
}
