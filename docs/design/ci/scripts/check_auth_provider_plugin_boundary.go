package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type fileRule struct {
	path           string
	required       []string
	forbiddenRegex []*regexp.Regexp
	forbiddenText  []string
	mustNotExist   bool
}

func main() {
	privateNaming := []*regexp.Regexp{}

	rules := []fileRule{
		{
			path: "internal/api/handlers/server_admin_catalog.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				"adminglobal.List()",
				"adminglobal.Resolve(authType)",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"providerregistry.ResolveAuthProviderAdminAdapter(",
				"providerregistry.ListAuthProviderAdminAdapterTypes()",
			},
			forbiddenRegex: []*regexp.Regexp{
				regexp.MustCompile(`authType\s*==\s*"(oidc|ldap|wecom)"`),
				regexp.MustCompile(`case\s+"(oidc|ldap|wecom)"`),
			},
		},
		{
			path: "internal/api/handlers/server_auth_external.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				`"kv-shepherd.io/shepherd/internal/provider/admincontract"`,
				`"kv-shepherd.io/shepherd/internal/provider/runtimecontract"`,
				"s.resolveLoginAuthProviderAdapter(",
				"adminglobal.Resolve(providerRow.AuthType)",
				"runtimeCapability.StartLogin(",
				"runtimeCapability.CompleteLogin(",
				"credentialCapability.AuthenticateCredentials(",
				"txExternalAuth.UpsertExternalUser(",
				"txExternalAuth.RecordLogin(",
			},
			forbiddenText: []string{
				"department",
				"base_dn",
				"selected_fields",
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"providerregistry.ResolveAuthProviderAdminAdapter(",
				"providerregistry.AuthRuntimeCapability",
				"providerregistry.AuthCredentialCapability",
				"providerregistry.AuthCallbackRequest",
				"providerregistry.AuthResult",
			},
			forbiddenRegex: []*regexp.Regexp{
				regexp.MustCompile(`provider(ID|Type|Ent)?\s*==\s*"(oidc|ldap|wecom)"`),
				regexp.MustCompile(`case\s+"(oidc|ldap|wecom)"`),
			},
		},
		{
			path: "internal/service/external_auth.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/runtimecontract"`,
				"normalizeExternalCohorts(",
				"syncObservedExternalCohorts(",
				"reconcileExternalCohortRBAC(",
				"UpsertExternalUser(",
			},
			forbiddenText: []string{
				"department",
				"groups",
				"base_dn",
				"selected_fields",
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.AuthResult",
				"provider.ExternalCohort",
			},
			forbiddenRegex: []*regexp.Regexp{
				regexp.MustCompile(`authType\s*==\s*"(oidc|ldap|wecom)"`),
				regexp.MustCompile(`case\s+"(oidc|ldap|wecom)"`),
			},
		},
		{
			path: "internal/service/vm_service.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/infracontract"`,
				"infra infracontract.InfrastructureProvider",
				"opts infracontract.ListOptions",
				"s.infra.(infracontract.ProvisioningQueryProvider)",
				"s.infra.(infracontract.NamespaceProvisioner)",
			},
			forbiddenText: []string{
				"provider.InfrastructureProvider",
				"provider.ListOptions",
				"provider.ProvisioningQueryProvider",
				"provider.NamespaceProvisioner",
			},
		},
		{
			path: "internal/service/vm_provisioning.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/infracontract"`,
				"s.infra.(infracontract.ProvisioningQueryProvider)",
			},
			forbiddenText: []string{
				"provider.ProvisioningQueryProvider",
			},
		},
		{
			path: "internal/service/vm_clone_source.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/infracontract"`,
				"s.infra.(infracontract.PVCClonePreflightProvider)",
				"validator infracontract.PVCClonePreflightProvider",
			},
			forbiddenText: []string{
				"provider.PVCClonePreflightProvider",
			},
		},
		{
			path: "internal/service/external_cohort_rbac.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/runtimecontract"`,
				"[]runtimecontract.ExternalCohort",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.ExternalCohort",
			},
		},
		{
			path: "internal/api/handlers/server_admin_directory_sync.go",
			required: []string{
				"s.resolveDirectorySyncCapability(",
				"capability.DescribeDirectorySync()",
				"capability.PreviewDirectorySync(",
				"req.ProviderRequest",
				`"kv-shepherd.io/shepherd/internal/edge/authworkspace/directoryview"`,
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				`"kv-shepherd.io/shepherd/internal/provider/directorycontract"`,
				"adminglobal.Resolve(providerRow.AuthType)",
			},
			forbiddenRegex: []*regexp.Regexp{
				regexp.MustCompile(`providerRequest\s*\[\s*"(departments|base_dn|selected_fields|groups)"\s*\]`),
				regexp.MustCompile(`case\s+"(oidc|ldap|azure|feishu)"`),
			},
			forbiddenText: []string{
				".metadata",
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"providerregistry.ResolveAuthProviderAdminAdapter(",
			},
		},
		{
			path: "internal/api/handlers/server_admin_runtime.go",
			required: []string{
				"runtimeview.BuildRuntimeDescriptor(",
				`"kv-shepherd.io/shepherd/internal/edge/authworkspace/runtimeview"`,
			},
			forbiddenRegex: []*regexp.Regexp{
				regexp.MustCompile(`case\s+"(oidc|ldap|azure|feishu)"`),
				regexp.MustCompile(`provider(ID|Type|Ent)?\s*==\s*"(oidc|ldap|azure|feishu)"`),
			},
			forbiddenText: []string{
				"department",
				"base_dn",
				"selected_fields",
			},
		},
		{
			path: "internal/api/handlers/server_admin.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/capabilityutil"`,
				"capabilityutil.HasAllCapabilities(",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.HasAllCapabilities(",
			},
		},
		{
			path: "internal/api/handlers/server_vm_capability.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/capabilityutil"`,
				"capabilityutil.HasAllCapabilities(",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.HasAllCapabilities(",
			},
		},
		{
			path: "internal/service/directory_sync.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/directorycontract"`,
				`"kv-shepherd.io/shepherd/internal/provider/runtimecontract"`,
				"CanonicalizePreview(",
				"ClassifyRecord(",
				"ApplyRecord(",
			},
			forbiddenRegex: []*regexp.Regexp{
				regexp.MustCompile(`providerRequest\s*\[\s*"(departments|base_dn|selected_fields|groups)"\s*\]`),
				regexp.MustCompile(`RequestSnapshot\s*\[\s*"(departments|base_dn|selected_fields|groups)"\s*\]`),
				regexp.MustCompile(`case\s+"(oidc|ldap|azure|feishu)"`),
			},
			forbiddenText: []string{
				".metadata",
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.DirectoryUserRecord",
				"provider.DirectoryConflict",
				"provider.DirectoryJoinKeyType",
				"provider.DirectoryAction",
				"provider.AuthResult",
			},
		},
		{
			path: "internal/jobs/directory_sync_worker.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				`"kv-shepherd.io/shepherd/internal/provider/directorycontract"`,
				"adminglobal.Resolve(authProviderRow.AuthType)",
				"directorycontract.DirectoryActionSummary",
				"directorycontract.DirectoryJoinKeyType(jobRow.JoinKeyType)",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.ResolveAuthProviderAdminAdapter(",
				"provider.DirectorySyncCapability",
				"provider.DirectoryActionSummary",
				"provider.DirectoryJoinKeyType(",
			},
		},
		{
			path: "internal/jobs/vm_create.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/infracontract"`,
				"infracontract.ListOptions{",
			},
			forbiddenText: []string{
				"provider.ListOptions{",
			},
		},
		{
			path: "internal/jobs/vm_status_sync.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/infracontract"`,
				"infracontract.ListOptions{",
			},
			forbiddenText: []string{
				"provider.ListOptions{",
			},
		},
		{
			path: "internal/jobs/directory_enrichment_schedule_scan.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				`"kv-shepherd.io/shepherd/internal/provider/directorycontract"`,
				"adminglobal.Resolve(authProviderRow.AuthType)",
				"directorycontract.DirectorySyncCapability",
				"directorycontract.ScheduledDirectoryEnrichmentCapability",
				"directorycontract.NormalizeScheduledDirectoryEnrichmentPlan(plan)",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.ResolveAuthProviderAdminAdapter(",
				"provider.DirectorySyncCapability",
				"provider.ScheduledDirectoryEnrichmentCapability",
				"provider.NormalizeScheduledDirectoryEnrichmentPlan(",
			},
		},
		{
			path: "internal/api/handlers/vm_live_status.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/infracontract"`,
				"infracontract.ListOptions{",
			},
			forbiddenText: []string{
				"provider.ListOptions{}",
			},
		},
		{
			path: "internal/provider/capability.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/capabilityutil"`,
				"return capabilityutil.HasAllCapabilities(clusterFeatures, required)",
			},
		},
		{
			path: "internal/provider/interface.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/infracontract"`,
				"type InfrastructureProvider = infracontract.InfrastructureProvider",
				"type ListOptions = infracontract.ListOptions",
			},
			forbiddenText: []string{
				"type InfrastructureProvider interface {",
				"type ListOptions struct {",
			},
		},
		{
			path: "internal/provider/infracontract/contract.go",
			required: []string{
				"type InfrastructureProvider interface {",
				"type ProvisioningQueryProvider interface {",
				"type PVCClonePreflightProvider interface {",
				"type ListOptions struct {",
			},
		},
		{
			path: "internal/provider/capabilityutil/capability.go",
			required: []string{
				"package capabilityutil",
				"func HasAllCapabilities(clusterFeatures, required []string) bool {",
			},
		},
		{
			path: "internal/provider/auth.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/runtimecontract"`,
				"type AuthResult = runtimecontract.AuthResult",
				"type AuthRuntimeCapability = runtimecontract.AuthRuntimeCapability",
			},
			forbiddenText: []string{
				"type AuthResult struct {",
				"type AuthStartRequest struct {",
				"type AuthRuntimeCapability interface {",
			},
		},
		{
			path: "internal/governance/approval/provider_router.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/approvalcontract"`,
				"provider approvalcontract.ApprovalProvider",
				"req *approvalcontract.ApprovalRequest",
				"decision approvalcontract.ApprovalDecision",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.ApprovalProvider",
				"provider.ApprovalRequest",
				"provider.ApprovalDecision",
			},
		},
		{
			path: "internal/governance/approval/builtin/provider.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/approvalcontract"`,
				"var _ approvalcontract.ApprovalProvider = (*Provider)(nil)",
				"req *approvalcontract.ApprovalRequest",
				"decision approvalcontract.ApprovalDecision",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.ApprovalProvider",
				"provider.ApprovalRequest",
				"provider.ApprovalResponse",
				"provider.ApprovalDecision",
			},
		},
		{
			path: "internal/api/handlers/server_approval.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/approvalcontract"`,
				"approvalcontract.ApprovalDecision{",
				"approvalcontract.ApprovalExecutionOptions{",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.ApprovalDecision",
				"provider.ApprovalExecutionOptions",
			},
		},
		{
			path: "internal/api/handlers/server_vm.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/approvalcontract"`,
				"&approvalcontract.ApprovalRequest{",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.ApprovalRequest",
			},
		},
		{
			path: "internal/api/handlers/server_vm_modify.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/approvalcontract"`,
				"&approvalcontract.ApprovalRequest{",
			},
			forbiddenText: []string{
				"provider.ApprovalRequest",
			},
		},
		{
			path: "internal/api/handlers/server_vm_batch.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/approvalcontract"`,
				"&approvalcontract.ApprovalRequest{",
				"approvalcontract.ApprovalDecision{",
				"approvalcontract.ApprovalExecutionOptions{",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"provider.ApprovalRequest",
				"provider.ApprovalDecision",
				"provider.ApprovalExecutionOptions",
			},
		},
		{
			path: "internal/provider/approval.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/approvalcontract"`,
				"type ApprovalProvider = approvalcontract.ApprovalProvider",
				"type ApprovalRequest = approvalcontract.ApprovalRequest",
			},
			forbiddenText: []string{
				"type ApprovalProvider interface {",
				"type ApprovalRequest struct {",
				"type ApprovalResponse struct {",
				"type ApprovalDecision struct {",
				"type ApprovalExecutionOptions struct {",
			},
		},
		{
			path: "internal/provider/approvalcontract/contract.go",
			required: []string{
				"type ApprovalProvider interface {",
				"type ApprovalRequest struct {",
				"type ApprovalResponse struct {",
				"type ApprovalDecision struct {",
				"type ApprovalExecutionOptions struct {",
			},
		},
		{
			path: "internal/provider/notification.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/notificationcontract"`,
				"type NotificationProvider = notificationcontract.NotificationProvider",
				"type Notification = notificationcontract.Notification",
			},
			forbiddenText: []string{
				"type NotificationProvider interface {",
				"type Notification struct {",
			},
		},
		{
			path: "internal/provider/notificationcontract/contract.go",
			required: []string{
				"type NotificationProvider interface {",
				"type Notification struct {",
			},
		},
		{
			path: "internal/provider/auth_provider_config_codec.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/configcodec"`,
				"type AuthProviderConfigCodec = configcodec.AuthProviderConfigCodec",
				"func NewAuthProviderConfigCodec(encryptionKey []byte) *AuthProviderConfigCodec {",
			},
			forbiddenText: []string{
				"type AuthProviderConfigCodec struct {",
				"func (c *AuthProviderConfigCodec) EncryptForStorage(",
				"func (c *AuthProviderConfigCodec) DecryptForUse(",
				"func (c *AuthProviderConfigCodec) SanitizeForAPI(",
				"func (c *AuthProviderConfigCodec) MergeForUpdate(",
			},
		},
		{
			path: "internal/provider/configcodec/codec.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/admincontract"`,
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				"type AuthProviderConfigCodec struct {",
				"func NewAuthProviderConfigCodec(encryptionKey []byte) *AuthProviderConfigCodec {",
				"func (c *AuthProviderConfigCodec) EncryptForStorage(",
				"func (c *AuthProviderConfigCodec) DecryptForUse(",
				"func (c *AuthProviderConfigCodec) SanitizeForAPI(",
				"func (c *AuthProviderConfigCodec) MergeForUpdate(",
			},
		},
		{
			path: "internal/provider/admin.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/admincontract"`,
				"type AuthProviderAdminAdapter = admincontract.AuthProviderAdminAdapter",
			},
			forbiddenText: []string{
				"type AuthProviderTypeDescriptor struct {",
				"type AuthProviderSampleField struct {",
				"type AuthProviderAdminAdapter interface {",
				"var globalAuthProviderAdminRegistry = newAuthProviderAdminRegistry()",
				"func RegisterAuthProviderAdminAdapter(",
				"func ResolveAuthProviderAdminAdapter(",
				"func ListAuthProviderAdminAdapterTypes(",
			},
		},
		{
			path: "internal/provider/auth_provider_admin_global.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				"func init() {",
				"adminglobal.MustRegister(builtInAuthProviderAdapters()...)",
				"func RegisterAuthProviderAdminAdapter(adapter AuthProviderAdminAdapter) error {",
				"func ResolveAuthProviderAdminAdapter(authType string) AuthProviderAdminAdapter {",
				"func ListAuthProviderAdminAdapterTypes() []AuthProviderTypeDescriptor {",
			},
		},
		{
			path:         "internal/provider/auth_provider_admin_registry.go",
			mustNotExist: true,
		},
		{
			path: "internal/provider/adminglobal/global.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/admincontract"`,
				`"kv-shepherd.io/shepherd/internal/provider/adminregistry"`,
				"var globalRegistry = adminregistry.New()",
				"func Register(adapter admincontract.AuthProviderAdminAdapter) error {",
				"func MustRegister(adapters ...admincontract.AuthProviderAdminAdapter) {",
				"func Resolve(authType string) admincontract.AuthProviderAdminAdapter {",
				"func List() []admincontract.AuthProviderTypeDescriptor {",
			},
			forbiddenText: []string{
				"newLDAPAuthProviderAdapter(",
			},
		},
		{
			path: "internal/provider/auth_provider_admin_builtins.go",
			required: []string{
				"newOIDCBuiltInAuthProviderAdapter()",
				"newLDAPBuiltInAuthProviderAdapter()",
				"newWeComBuiltInAuthProviderAdapter()",
			},
			forbiddenText: []string{
				"configSchema:",
				"oidcSchema :=",
				"newLDAPAuthProviderAdapter(",
			},
		},
		{
			path: "internal/provider/auth_provider_oidc_admin.go",
			required: []string{
				"func newOIDCBuiltInAuthProviderAdapter() AuthProviderAdminAdapter {",
				"func (a *oidcAuthProviderAdapter) StartLogin(",
				"func (a *oidcAuthProviderAdapter) CompleteLogin(",
			},
		},
		{
			path: "internal/provider/auth_provider_ldap.go",
			required: []string{
				"func newLDAPBuiltInAuthProviderAdapter() AuthProviderAdminAdapter {",
			},
		},
		{
			path: "internal/provider/auth_provider_ldap_schema.go",
			required: []string{
				"func ldapAuthProviderSchema() map[string]interface{} {",
			},
		},
		{
			path: "internal/provider/auth_provider_oidc_schema.go",
			required: []string{
				"func oidcAuthProviderSchema() map[string]interface{} {",
			},
		},
		{
			path: "internal/provider/auth_provider_wecom_schema.go",
			required: []string{
				"func weComAuthProviderSchema() map[string]interface{} {",
			},
		},
		{
			path: "internal/provider/directory_sync.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/directorycontract"`,
				"type DirectorySyncDescriptor = directorycontract.DirectorySyncDescriptor",
				"type DirectorySyncCapability = directorycontract.DirectorySyncCapability",
			},
			forbiddenText: []string{
				"type DirectorySyncDescriptor struct {",
				"type DirectoryUserRecord struct {",
				"type ScheduledDirectoryEnrichmentPlan struct {",
				"type DirectorySyncCapability interface {",
				"func externalCohortsFromStringValues(",
			},
		},
		{
			path: "internal/provider/auth_provider_cohorts.go",
			required: []string{
				"func externalCohortsFromStringValues(kind string, values []string) []ExternalCohort {",
			},
		},
		{
			path: "pkg/authproviderplugin/runtime.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/runtimecontract"`,
				"type RuntimeCapability = runtimecontract.AuthRuntimeCapability",
			},
			forbiddenText: []string{
				"internalprovider.AuthResult",
				"internalprovider.AuthRuntimeCapability",
			},
			forbiddenRegex: privateNaming,
		},
		{
			path: "web/src/features/admin-auth-providers/components/AdminAuthProvidersContent.tsx",
			required: []string{
				"SchemaConfigForm",
				"cohort-mapping-create-button",
			},
			forbiddenText: []string{
				"group-mapping-create-button",
				"config_text",
			},
		},
		{
			path: "web/src/features/admin-auth-providers/hooks/useAdminAuthProvidersController.ts",
			required: []string{
				"api.GET(\"/admin/auth-provider-types\")",
				"extractConfigObject(",
			},
			forbiddenText: []string{
				"auth_type: 'oidc'",
				"auth_type: \"oidc\"",
				"AUTH_PROVIDER_TYPE_OPTIONS",
				"config_text",
			},
		},
		{
			path: "web/src/features/admin-auth-providers/types.ts",
			forbiddenText: []string{
				"AUTH_PROVIDER_TYPE_OPTIONS",
			},
		},
		{
			path: "web/tests/e2e/admin-flow-live.spec.ts",
			required: []string{
				"getAuthProviderSample",
				"listAuthProviderCohorts",
				"listAuthProviderCohortMappings",
				"/api/v1/admin/auth-providers/${providerID}/cohorts",
				"/api/v1/admin/auth-providers/${providerID}/cohort-mappings",
				"getByLabel(/issuer url/i)",
				"getByLabel(/client id/i)",
				"getByLabel(/client secret/i)",
			},
			forbiddenText: []string{
				"group-mappings",
				"IdPGroupMapping",
				"syncAuthProviderGroups",
				"AuthProviderGroupSyncResponse",
				"provider config",
			},
		},
		{
			path: "web/tests/e2e/admin-extended-live.spec.ts",
			required: []string{
				"getAuthProviderDirectoryDescriptor returns unsupported for non-directory providers",
				"createAuthProviderCohortMapping",
				"ExternalCohortMapping",
				"toBe(501)",
				"/cohort-mappings",
				"getByLabel(/issuer url/i)",
			},
			forbiddenText: []string{
				"group-mappings",
				"IdPGroupMapping",
				"syncAuthProviderGroups",
				"AuthProviderGroupSyncResponse",
				"provider config",
			},
		},
		{
			path: "web/tests/e2e/master-flow-live.spec.ts",
			required: []string{
				"Stage 2.B – listAuthProviderTypes: auth provider type list conforms to AuthProviderTypeList schema",
				"Stage 2.B – listAuthProviders: auth provider list conforms to AuthProviderList schema",
				"/api/v1/admin/auth-provider-types",
				"/api/v1/admin/auth-providers",
			},
		},
		{
			path: "api/openapi.yaml",
			forbiddenText: []string{
				"OIDC/LDAP authentication provider management",
				"Create authentication provider (OIDC/LDAP)",
				"Update authentication provider (OIDC/LDAP)",
			},
		},
		{
			path: "plugins/authprovider/example/plugin.go",
			required: []string{
				`"kv-shepherd.io/shepherd/pkg/authproviderplugin"`,
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/`,
			},
			forbiddenRegex: privateNaming,
		},
		{
			path: "plugins/authprovider/template/template.go",
			required: []string{
				`"kv-shepherd.io/shepherd/pkg/authproviderplugin"`,
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/`,
			},
			forbiddenRegex: privateNaming,
		},
		{
			path: "tools/sdk-smoke/authproviderplugin-external/sdk_test.go",
			required: []string{
				`"kv-shepherd.io/shepherd/pkg/authproviderplugin"`,
				"authproviderplugin.RegisterAdminAdapter(",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/`,
			},
			forbiddenRegex: privateNaming,
		},
		{
			path: "tools/sdk-smoke/authproviderplugin-external/plugins/example/plugin.go",
			required: []string{
				`"kv-shepherd.io/shepherd/pkg/authproviderplugin"`,
				"authproviderplugin.MustRegisterAdminAdapter(",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/`,
			},
			forbiddenRegex: privateNaming,
		},
		{
			path: "tools/sdk-smoke/authproviderplugin-external/cmd/server-enterprise/main.go",
			required: []string{
				`"kv-shepherd.io/shepherd/pkg/serverbootstrap"`,
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/`,
			},
			forbiddenRegex: privateNaming,
		},
		{
			path: "pkg/authproviderplugin/admin.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				`"kv-shepherd.io/shepherd/internal/provider/admincontract"`,
				`"kv-shepherd.io/shepherd/internal/provider/directorycontract"`,
				"type AdminAdapter = admincontract.AuthProviderAdminAdapter",
			},
			forbiddenText: []string{
				"internalprovider.RegisterAuthProviderAdminAdapter(",
				"internalprovider.ListAuthProviderAdminAdapterTypes(",
				"internalprovider.AuthProviderTypeDescriptor",
				"internalprovider.AuthProviderSampleField",
				"internalprovider.AuthProviderAdminAdapter",
			},
			forbiddenRegex: privateNaming,
		},
		{
			path: "internal/edge/authworkspace/runtimeview/runtime.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				`"kv-shepherd.io/shepherd/internal/provider/admincontract"`,
				`"kv-shepherd.io/shepherd/internal/provider/runtimecontract"`,
				"adminglobal.Resolve(",
				"runtimecontract.AuthRuntimeCapability",
				"runtimecontract.AuthRuntimeDescriber",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"providerregistry.ResolveAuthProviderAdminAdapter(",
			},
		},
		{
			path: "internal/api/handlers/server.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/configcodec"`,
				"authProviderConfig   *configcodec.AuthProviderConfigCodec",
				"authProviderConfig:   configcodec.NewAuthProviderConfigCodec(deps.EncryptionKey),",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"authProviderConfig   *provider.AuthProviderConfigCodec",
				"provider.NewAuthProviderConfigCodec(",
			},
		},
		{
			path: "internal/edge/authworkspace/directoryview/directory.go",
			required: []string{
				`"kv-shepherd.io/shepherd/internal/provider/adminglobal"`,
				`"kv-shepherd.io/shepherd/internal/provider/directorycontract"`,
				"adminglobal.Resolve(",
				"directorycontract.CloneDirectoryAttributes",
				"directorycontract.NormalizeScheduledDirectoryEnrichmentPlan",
			},
			forbiddenText: []string{
				`"kv-shepherd.io/shepherd/internal/provider"`,
				"providerregistry.ResolveAuthProviderAdminAdapter(",
			},
		},
		{
			path:           "pkg/authproviderplugin/runtime.go",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "plugins/authprovider/README.md",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "plugins/authprovider/template/README.md",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "docs/adr/ADR-0048-directory-sync-capability.md",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "docs/adr/ADR-0049-external-auth-runtime-jit-provisioning-and-external-cohort-rbac-mapping.md",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "docs/adr/ADR-0050-upstream-identity-assertion-runtime-provider.md",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "docs/adr/ADR-0051-scheduled-directory-enrichment.md",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "docs/design/notes/ADR-0048-directory-sync-capability.md",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "docs/design/notes/ADR-0049-external-auth-runtime-jit-provisioning-and-external-cohort-rbac-mapping.md",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "docs/design/notes/ADR-0050-upstream-identity-assertion-runtime-provider.md",
			forbiddenRegex: privateNaming,
		},
		{
			path:           "docs/design/notes/ADR-0051-scheduled-directory-enrichment.md",
			forbiddenRegex: privateNaming,
		},
	}

	var failures []string
	for _, rule := range rules {
		if rule.mustNotExist {
			if _, err := os.Stat(rule.path); err == nil {
				failures = append(failures, fmt.Sprintf("%s: file must not exist", rule.path))
			} else if !os.IsNotExist(err) {
				failures = append(failures, fmt.Sprintf("%s: stat failed: %v", rule.path, err))
			}
			continue
		}
		content, err := os.ReadFile(rule.path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: read failed: %v", rule.path, err))
			continue
		}
		text := string(content)

		for _, req := range rule.required {
			if !strings.Contains(text, req) {
				failures = append(failures, fmt.Sprintf("%s: missing required fragment %q", rule.path, req))
			}
		}
		for _, forbidden := range rule.forbiddenText {
			if strings.Contains(text, forbidden) {
				failures = append(failures, fmt.Sprintf("%s: found forbidden text %q", rule.path, forbidden))
			}
		}
		for _, re := range rule.forbiddenRegex {
			if match := re.FindString(text); match != "" {
				failures = append(failures, fmt.Sprintf("%s: found forbidden provider-specific branch %q", rule.path, match))
			}
		}
	}

	allowedRootProviderImports := map[string]struct{}{
		filepath.Clean("internal/app/bootstrap.go"):                 {},
		filepath.Clean("internal/app/lifecycle.go"):                 {},
		filepath.Clean("internal/app/modules/infrastructure.go"):    {},
		filepath.Clean("internal/app/modules/server_deps.go"):       {},
		filepath.Clean("internal/api/handlers/server_vm_modify.go"): {},
		filepath.Clean("internal/jobs/vm_modify.go"):                {},
		filepath.Clean("internal/service/vm_provisioning.go"):       {},
		filepath.Clean("internal/service/vm_service.go"):            {},
	}
	rootProviderImport := `"kv-shepherd.io/shepherd/internal/provider"`
	rootImportCheckRoots := []string{
		"internal/app",
		"internal/api",
		"internal/service",
		"internal/jobs",
		"internal/governance",
	}
	for _, root := range rootImportCheckRoots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: walk failed: %v", path, err))
				return nil
			}
			if info == nil || info.IsDir() {
				return nil
			}
			clean := filepath.Clean(path)
			if !strings.HasSuffix(clean, ".go") || strings.HasSuffix(clean, "_test.go") {
				return nil
			}
			content, readErr := os.ReadFile(clean)
			if readErr != nil {
				failures = append(failures, fmt.Sprintf("%s: read failed: %v", clean, readErr))
				return nil
			}
			if !strings.Contains(string(content), rootProviderImport) {
				return nil
			}
			if _, ok := allowedRootProviderImports[clean]; !ok {
				failures = append(failures, fmt.Sprintf("%s: unexpected root provider import; depend on a narrower contract/utility package instead", clean))
			}
			return nil
		})
	}

	if len(failures) > 0 {
		_, _ = os.Stdout.WriteString("FAIL: auth provider plugin boundary check failed\n")
		for _, item := range failures {
			_, _ = os.Stdout.WriteString(fmt.Sprintf(" - %s\n", item))
		}
		_, _ = os.Stdout.WriteString("Rule: auth-provider core must stay plugin-standard and must not hardcode provider-specific branches in runtime paths.\n")
		os.Exit(1)
	}

	_, _ = os.Stdout.WriteString("OK: auth provider plugin boundary check passed\n")
}
