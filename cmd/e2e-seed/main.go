// Package main seeds deterministic fixtures for live end-to-end tests.
//
// This command is test-environment only and is intentionally idempotent.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"k8s.io/client-go/tools/clientcmd"

	"kv-shepherd.io/shepherd/ent"
	entapprovalticket "kv-shepherd.io/shepherd/ent/approvalticket"
	entauthprovider "kv-shepherd.io/shepherd/ent/authprovider"
	entbatchapprovalticket "kv-shepherd.io/shepherd/ent/batchapprovalticket"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entdomainevent "kv-shepherd.io/shepherd/ent/domainevent"
	entinstancesize "kv-shepherd.io/shepherd/ent/instancesize"
	entnamespaceregistry "kv-shepherd.io/shepherd/ent/namespaceregistry"
	entnotification "kv-shepherd.io/shepherd/ent/notification"
	entrole "kv-shepherd.io/shepherd/ent/role"
	entrolebinding "kv-shepherd.io/shepherd/ent/rolebinding"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entsystem "kv-shepherd.io/shepherd/ent/system"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entuser "kv-shepherd.io/shepherd/ent/user"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/infrastructure"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	defaultAdminUsername = "e2e-admin"
	defaultAdminPassword = "e2e-admin-123" // #nosec G101 -- test fixture credential for local e2e seeding only.
	defaultAdminEmail    = "e2e-admin@localhost"

	defaultNamespaceName = "e2e-test"
	defaultClusterName   = "e2e-cluster"
	defaultClusterAPIURL = "https://e2e.invalid"
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
	ClusterAPIURL string
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

	fx := loadFixtureConfig()
	client := db.EntClient

	adminID, err := ensureAdminUser(ctx, client, fx)
	if err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}
	if bindErr := ensureAdminRoleBinding(ctx, client, adminID); bindErr != nil {
		return fmt.Errorf("ensure admin role binding: %w", bindErr)
	}

	if nsErr := ensureNamespaceRegistry(ctx, client, fx); nsErr != nil {
		return fmt.Errorf("ensure namespace: %w", nsErr)
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
	if tplErr := ensureTemplate(ctx, client, fx); tplErr != nil {
		return fmt.Errorf("ensure template: %w", tplErr)
	}
	if sizeErr := ensureInstanceSize(ctx, client, fx); sizeErr != nil {
		return fmt.Errorf("ensure instance size: %w", sizeErr)
	}
	if runningVMErr := ensureVM(ctx, client, fx.RunningVMID, "vm-live", "01", entvm.StatusRUNNING, fx.NamespaceName, clusterID, serviceID, fx.AdminUsername); runningVMErr != nil {
		return fmt.Errorf("ensure running vm: %w", runningVMErr)
	}
	if stoppedVMErr := ensureVM(ctx, client, fx.StoppedVMID, "vm-stopped", "02", entvm.StatusSTOPPED, fx.NamespaceName, clusterID, serviceID, fx.AdminUsername); stoppedVMErr != nil {
		return fmt.Errorf("ensure stopped vm: %w", stoppedVMErr)
	}

	// ── Phase 2: Extended fixtures (auth provider, extra user, notifications, approvals) ──

	if providerErr := ensureAuthProvider(ctx, client); providerErr != nil {
		return fmt.Errorf("ensure auth provider: %w", providerErr)
	}

	secondUserID, err := ensureSecondUser(ctx, client)
	if err != nil {
		return fmt.Errorf("ensure second user: %w", err)
	}
	_ = secondUserID // Used for member management tests; ID logged below

	if approvalErr := ensureApprovalTickets(ctx, client); approvalErr != nil {
		return fmt.Errorf("ensure approval tickets: %w", approvalErr)
	}

	if notifyErr := ensureNotifications(ctx, client, adminID); notifyErr != nil {
		return fmt.Errorf("ensure notifications: %w", notifyErr)
	}

	if batchErr := ensureBatchApprovalTickets(ctx, client, fx.AdminUsername, clusterID, fx.NamespaceName); batchErr != nil {
		return fmt.Errorf("ensure batch approval tickets: %w", batchErr)
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
		ClusterAPIURL: envOrDefault("E2E_CLUSTER_API_SERVER", defaultClusterAPIURL),
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

type clusterSeedInput struct {
	Kubeconfig []byte
	APIServer  string
	Status     entcluster.Status
}

func resolveClusterSeedInput(fx fixtureConfig) (clusterSeedInput, error) {
	const fallbackStub = "apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n"
	rawB64 := strings.TrimSpace(os.Getenv("E2E_KUBECONFIG_B64"))
	if rawB64 == "" {
		return clusterSeedInput{
			Kubeconfig: []byte(fallbackStub),
			APIServer:  fx.ClusterAPIURL,
			Status:     entcluster.StatusUNREACHABLE,
		}, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return clusterSeedInput{}, fmt.Errorf("decode E2E_KUBECONFIG_B64: %w", err)
	}
	if strings.TrimSpace(string(decoded)) == "" {
		return clusterSeedInput{}, fmt.Errorf("E2E_KUBECONFIG_B64 decoded to empty content")
	}

	cfg, err := clientcmd.Load(decoded)
	if err != nil {
		return clusterSeedInput{}, fmt.Errorf("parse kubeconfig from E2E_KUBECONFIG_B64: %w", err)
	}

	apiServer := strings.TrimSpace(fx.ClusterAPIURL)
	if apiServer == "" || apiServer == defaultClusterAPIURL {
		for _, cluster := range cfg.Clusters {
			if server := strings.TrimSpace(cluster.Server); server != "" {
				apiServer = server
				break
			}
		}
	}
	if apiServer == "" {
		return clusterSeedInput{}, fmt.Errorf("kubeconfig in E2E_KUBECONFIG_B64 does not contain any cluster server")
	}

	return clusterSeedInput{
		Kubeconfig: decoded,
		APIServer:  apiServer,
		Status:     entcluster.StatusHEALTHY,
	}, nil
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
	clusterInput, err := resolveClusterSeedInput(fx)
	if err != nil {
		return "", err
	}

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
			SetAPIServerURL(clusterInput.APIServer).
			SetEncryptedKubeconfig(clusterInput.Kubeconfig).
			SetStatus(clusterInput.Status).
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
		SetAPIServerURL(clusterInput.APIServer).
		SetEncryptedKubeconfig(clusterInput.Kubeconfig).
		SetStatus(clusterInput.Status).
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
		Where(enttemplate.NameEQ(fx.TemplateName)).
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
			SetDescription("e2e template - ContainerDisk boot").
			SetSourceType("image").
			SetImageURL("quay.io/containerdisks/ubuntu:22.04").
			SetOsFamily("linux").
			SetOsVersion("ubuntu-22.04").
			SetEnabled(true).
			SetCreatedBy("e2e-seed").
			Save(ctx)
		return createErr
	}

	_, err = client.Template.UpdateOneID(obj.ID).
		SetDisplayName("E2E Template").
		SetDescription("e2e template - ContainerDisk boot").
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
			SetMemoryGi(4.0).
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
		SetMemoryGi(4.0).
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

// ── Extended seed fixtures ────────────────────────────────────────────────────

const (
	defaultAuthProviderName = "e2e-ldap"
	defaultSecondUsername   = "e2e-user"
	defaultSecondPassword   = "e2e-user-123" // #nosec G101 -- test fixture credential for local e2e seeding only.
	defaultSecondEmail      = "e2e-user@localhost"
)

// ensureAuthProvider creates a minimal LDAP-style auth provider for tests
// that exercise operationIds: updateAuthProvider, testAuthProviderConnection,
// syncAuthProviderGroups, getAuthProviderSample, CRUD group-mappings.
func ensureAuthProvider(ctx context.Context, client *ent.Client) error {
	ldapConfig := map[string]interface{}{
		"host":      "ldap.e2e.invalid",
		"port":      389,
		"base_dn":   "dc=e2e,dc=test",
		"bind_dn":   "cn=admin,dc=e2e,dc=test",
		"bind_pass": "e2e-secret",
		"tls":       false,
	}

	obj, err := client.AuthProvider.Query().
		Where(entauthprovider.NameEQ(defaultAuthProviderName)).
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return err
		}
		id, _ := uuid.NewV7()
		_, createErr := client.AuthProvider.Create().
			SetID(id.String()).
			SetName(defaultAuthProviderName).
			SetAuthType("ldap").
			SetConfig(ldapConfig).
			SetEnabled(true).
			SetSortOrder(0).
			SetCreatedBy("e2e-seed").
			Save(ctx)
		return createErr
	}

	_, err = client.AuthProvider.UpdateOneID(obj.ID).
		SetConfig(ldapConfig).
		SetEnabled(true).
		Save(ctx)
	return err
}

// ensureSecondUser creates a non-admin user needed for member management tests
// (addSystemMember, updateSystemMemberRole, deleteSystemMember require ≥2 users).
func ensureSecondUser(ctx context.Context, client *ent.Client) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(defaultSecondPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	user, err := client.User.Query().Where(entuser.UsernameEQ(defaultSecondUsername)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return "", err
		}
		id, _ := uuid.NewV7()
		created, createErr := client.User.Create().
			SetID(id.String()).
			SetUsername(defaultSecondUsername).
			SetEmail(defaultSecondEmail).
			SetDisplayName("E2E Regular User").
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
		SetEmail(defaultSecondEmail).
		SetDisplayName("E2E Regular User").
		SetPasswordHash(string(hash)).
		SetForcePasswordChange(false).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		return "", err
	}
	return updated.ID, nil
}

// ensureApprovalTickets creates PENDING approval tickets for tests that exercise
// approveTicket, rejectTicket, cancelTicket, submitApprovalBatch.
// Each ticket requires a DomainEvent (ADR-0009 claim-check pattern).
func ensureApprovalTickets(ctx context.Context, client *ent.Client) error {
	// We need 3 PENDING tickets: one for approve, one for reject, one for cancel
	for i, suffix := range []string{"approve", "reject", "cancel"} {
		eventID := fmt.Sprintf("evt-e2e-seed-%s", suffix)
		ticketID := fmt.Sprintf("tkt-e2e-seed-%s", suffix)

		// Ensure DomainEvent exists
		exists, err := client.DomainEvent.Query().
			Where(entdomainevent.IDEQ(eventID)).
			Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			payload, _ := json.Marshal(map[string]interface{}{
				"vm_name":          fmt.Sprintf("e2e-vm-%s", suffix),
				"instance":         fmt.Sprintf("%02d", i+10),
				"template_id":      "e2e-template",
				"instance_size_id": "e2e-small",
				"reason":           fmt.Sprintf("E2E seed ticket for %s test", suffix),
			})
			_, createEventErr := client.DomainEvent.Create().
				SetID(eventID).
				SetEventType("VM_CREATION_REQUESTED").
				SetAggregateType("vm").
				SetAggregateID(fmt.Sprintf("vm-e2e-%s", suffix)).
				SetPayload(payload).
				SetStatus(entdomainevent.StatusPENDING).
				SetCreatedBy("e2e-seed").
				Save(ctx)
			if createEventErr != nil {
				return fmt.Errorf("create domain event %s: %w", eventID, createEventErr)
			}
		}

		// Ensure ApprovalTicket exists and is PENDING
		ticketExists, err := client.ApprovalTicket.Query().
			Where(entapprovalticket.IDEQ(ticketID)).
			Exist(ctx)
		if err != nil {
			return err
		}
		if !ticketExists {
			_, createTicketErr := client.ApprovalTicket.Create().
				SetID(ticketID).
				SetEventID(eventID).
				SetOperationType(entapprovalticket.OperationTypeCREATE).
				SetStatus(entapprovalticket.StatusPENDING).
				SetRequester("e2e-seed").
				SetReason(fmt.Sprintf("E2E seed ticket for %s test", suffix)).
				Save(ctx)
			if createTicketErr != nil {
				return fmt.Errorf("create approval ticket %s: %w", ticketID, createTicketErr)
			}
		} else {
			// Reset to PENDING if it was previously approved/rejected during a test run
			_, err := client.ApprovalTicket.UpdateOneID(ticketID).
				SetStatus(entapprovalticket.StatusPENDING).
				ClearApprover().
				ClearRejectReason().
				Save(ctx)
			if err != nil {
				return fmt.Errorf("reset approval ticket %s: %w", ticketID, err)
			}
		}
	}
	return nil
}

// ensureNotifications creates unread notifications for the admin user.
// markNotificationRead and markAllNotificationsRead tests require at least one
// unread notification.
func ensureNotifications(ctx context.Context, client *ent.Client, adminID string) error {
	notifID := "notif-e2e-seed-01"

	exists, err := client.Notification.Query().
		Where(entnotification.IDEQ(notifID)).
		Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		// Reset to unread for test re-runs
		_, err := client.Notification.UpdateOneID(notifID).
			SetRead(false).
			ClearReadAt().
			Save(ctx)
		return err
	}

	// Create two unread notifications (one for markNotificationRead, one spare)
	for _, n := range []struct {
		id    string
		title string
		nType entnotification.Type
	}{
		{notifID, "VM creation approved", entnotification.TypeAPPROVAL_COMPLETED},
		{"notif-e2e-seed-02", "New VM request pending", entnotification.TypeAPPROVAL_PENDING},
	} {
		nExists, nErr := client.Notification.Query().
			Where(entnotification.IDEQ(n.id)).
			Exist(ctx)
		if nErr != nil {
			return nErr
		}
		if nExists {
			// Reset to unread
			_, _ = client.Notification.UpdateOneID(n.id).
				SetRead(false).
				ClearReadAt().
				Save(ctx)
			continue
		}
		_, createErr := client.Notification.Create().
			SetID(n.id).
			SetType(n.nType).
			SetTitle(n.title).
			SetMessage(fmt.Sprintf("Seeded by e2e-seed for notification testing: %s", n.title)).
			SetResourceType("approval_ticket").
			SetResourceID("tkt-e2e-seed-approve").
			SetRead(false).
			SetUserID(adminID).
			Save(ctx)
		if createErr != nil {
			return fmt.Errorf("create notification %s: %w", n.id, createErr)
		}
	}
	return nil
}

// ensureBatchApprovalTickets creates batch operation records for tests that
// exercise retryVMBatch (needs a FAILED batch) and cancelVMBatch (needs an
// IN_PROGRESS or PENDING_APPROVAL batch).
func ensureBatchApprovalTickets(
	ctx context.Context,
	client *ent.Client,
	createdBy string,
	clusterID string,
	namespaceName string,
) error {
	type childFixture struct {
		status       entapprovalticket.Status
		rejectReason string
	}
	type batchFixture struct {
		id                 string
		parentEventStatus  entdomainevent.Status
		parentTicketStatus entapprovalticket.Status
		projectionStatus   entbatchapprovalticket.Status
		children           []childFixture
	}

	serviceID := defaultServiceName
	if svc, err := client.Service.Query().Where(entservice.NameEQ(defaultServiceName)).Only(ctx); err == nil {
		serviceID = svc.ID
	}
	templateID := defaultTemplateName
	if tpl, err := client.Template.Query().Where(enttemplate.NameEQ(defaultTemplateName)).Only(ctx); err == nil {
		templateID = tpl.ID
	}
	sizeID := defaultSizeName
	if size, err := client.InstanceSize.Query().Where(entinstancesize.NameEQ(defaultSizeName)).Only(ctx); err == nil {
		sizeID = size.ID
	}
	selectedClusterID := strings.TrimSpace(clusterID)
	selectedStorageClass := ""
	if selectedClusterID != "" {
		if clusterObj, err := client.Cluster.Get(ctx, selectedClusterID); err == nil {
			selectedStorageClass = strings.TrimSpace(clusterObj.DefaultStorageClass)
		}
	}
	seedNamespace := strings.TrimSpace(namespaceName)
	if seedNamespace == "" {
		seedNamespace = defaultNamespaceName
	}

	batches := []batchFixture{
		{
			id:                 "batch-e2e-seed-failed",
			parentEventStatus:  entdomainevent.StatusFAILED,
			parentTicketStatus: entapprovalticket.StatusFAILED,
			projectionStatus:   entbatchapprovalticket.StatusFAILED,
			children: []childFixture{
				{status: entapprovalticket.StatusSUCCESS},
				{status: entapprovalticket.StatusFAILED, rejectReason: "seeded failed child for retry"},
				{status: entapprovalticket.StatusREJECTED, rejectReason: "seeded rejected child for retry"},
			},
		},
		{
			id:                 "batch-e2e-seed-pending",
			parentEventStatus:  entdomainevent.StatusPENDING,
			parentTicketStatus: entapprovalticket.StatusPENDING,
			projectionStatus:   entbatchapprovalticket.StatusIN_PROGRESS,
			children: []childFixture{
				{status: entapprovalticket.StatusPENDING},
				{status: entapprovalticket.StatusPENDING},
				{status: entapprovalticket.StatusPENDING},
				{status: entapprovalticket.StatusPENDING},
				{status: entapprovalticket.StatusPENDING},
			},
		},
	}

	for _, b := range batches {
		parentEventID := fmt.Sprintf("evt-%s-parent", b.id)
		parentReason := "Seeded by e2e-seed for batch operation testing"

		existingChildren, err := client.ApprovalTicket.Query().
			Where(entapprovalticket.ParentTicketIDEQ(b.id)).
			All(ctx)
		if err != nil {
			return fmt.Errorf("query existing child tickets for batch %s: %w", b.id, err)
		}
		staleEventIDs := make([]string, 0, len(existingChildren)+1)
		for _, child := range existingChildren {
			staleEventIDs = append(staleEventIDs, child.EventID)
		}
		staleEventIDs = append(staleEventIDs, parentEventID)

		if _, err := client.ApprovalTicket.Delete().
			Where(entapprovalticket.ParentTicketIDEQ(b.id)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete existing child tickets for batch %s: %w", b.id, err)
		}
		if err := client.ApprovalTicket.DeleteOneID(b.id).Exec(ctx); err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("delete parent ticket for batch %s: %w", b.id, err)
		}
		if _, err := client.DomainEvent.Delete().
			Where(entdomainevent.IDIn(staleEventIDs...)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete stale events for batch %s: %w", b.id, err)
		}
		if err := client.BatchApprovalTicket.DeleteOneID(b.id).Exec(ctx); err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("delete projection for batch %s: %w", b.id, err)
		}

		parentPayload, _ := json.Marshal(map[string]interface{}{
			"operation":    "CREATE",
			"request_id":   fmt.Sprintf("seed-%s", b.id),
			"reason":       parentReason,
			"submitted_by": createdBy,
			"items":        []map[string]interface{}{},
		})
		if _, err := client.DomainEvent.Create().
			SetID(parentEventID).
			SetEventType(string(domain.EventBatchCreateRequested)).
			SetAggregateType("batch").
			SetAggregateID(b.id).
			SetPayload(parentPayload).
			SetStatus(b.parentEventStatus).
			SetCreatedBy(createdBy).
			Save(ctx); err != nil {
			return fmt.Errorf("create parent event for batch %s: %w", b.id, err)
		}
		parentCreate := client.ApprovalTicket.Create().
			SetID(b.id).
			SetEventID(parentEventID).
			SetOperationType(entapprovalticket.OperationTypeCREATE).
			SetStatus(b.parentTicketStatus).
			SetRequester(createdBy).
			SetReason(parentReason)
		// For FAILED parent batches used by retryVMBatch, seed the selected
		// cluster context to match real approved CREATE tickets.
		if b.parentTicketStatus != entapprovalticket.StatusPENDING && selectedClusterID != "" {
			parentCreate = parentCreate.SetSelectedClusterID(selectedClusterID)
			if selectedStorageClass != "" {
				parentCreate = parentCreate.SetSelectedStorageClass(selectedStorageClass)
			}
		}
		if _, err := parentCreate.Save(ctx); err != nil {
			return fmt.Errorf("create parent ticket for batch %s: %w", b.id, err)
		}

		successCount := 0
		failedCount := 0
		pendingCount := 0
		for idx, child := range b.children {
			childEventID := fmt.Sprintf("evt-%s-child-%02d", b.id, idx+1)
			childTicketID := fmt.Sprintf("tkt-%s-child-%02d", b.id, idx+1)
			childReason := fmt.Sprintf("Seeded batch child %d for %s", idx+1, b.id)
			childPayload, _ := json.Marshal(map[string]interface{}{
				"requester_id":      createdBy,
				"service_id":        serviceID,
				"template_id":       templateID,
				"instance_size_id":  sizeID,
				"namespace":         seedNamespace,
				"reason":            childReason,
				"batch_seed_ticket": childTicketID,
			})

			var childEventStatus entdomainevent.Status
			switch child.status {
			case entapprovalticket.StatusSUCCESS:
				successCount++
				childEventStatus = entdomainevent.StatusCOMPLETED
			case entapprovalticket.StatusFAILED, entapprovalticket.StatusREJECTED:
				failedCount++
				childEventStatus = entdomainevent.StatusFAILED
			default:
				pendingCount++
				childEventStatus = entdomainevent.StatusPENDING
			}

			if _, err := client.DomainEvent.Create().
				SetID(childEventID).
				SetEventType(string(domain.EventVMCreationRequested)).
				SetAggregateType("vm").
				SetAggregateID(serviceID).
				SetPayload(childPayload).
				SetStatus(childEventStatus).
				SetCreatedBy(createdBy).
				Save(ctx); err != nil {
				return fmt.Errorf("create child event %s for batch %s: %w", childEventID, b.id, err)
			}

			childCreate := client.ApprovalTicket.Create().
				SetID(childTicketID).
				SetEventID(childEventID).
				SetOperationType(entapprovalticket.OperationTypeCREATE).
				SetStatus(child.status).
				SetRequester(createdBy).
				SetReason(childReason).
				SetParentTicketID(b.id)
			if child.rejectReason != "" {
				childCreate = childCreate.SetRejectReason(child.rejectReason)
			}
			if _, err := childCreate.Save(ctx); err != nil {
				return fmt.Errorf("create child ticket %s for batch %s: %w", childTicketID, b.id, err)
			}
		}

		if _, err := client.BatchApprovalTicket.Create().
			SetBatchType(entbatchapprovalticket.BatchTypeBATCH_CREATE).
			SetStatus(b.projectionStatus).
			SetChildCount(len(b.children)).
			SetSuccessCount(successCount).
			SetFailedCount(failedCount).
			SetPendingCount(pendingCount).
			SetID(b.id).
			SetCreatedBy(createdBy).
			SetReason(parentReason).
			Save(ctx); err != nil {
			return fmt.Errorf("create projection for batch %s: %w", b.id, err)
		}
	}

	return nil
}
