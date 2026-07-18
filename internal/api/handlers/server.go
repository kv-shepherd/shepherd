// Package handlers implements the generated ServerInterface (ADR-0021 contract-first).
//
// All methods satisfy the oapi-codegen generated ServerInterface.
// Route registration is handled by generated.RegisterHandlersWithOptions —
// handlers do NOT register their own routes.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/api/handlers
package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	approvalbuiltin "kv-shepherd.io/shepherd/internal/governance/approval/builtin"
	approvalregistry "kv-shepherd.io/shepherd/internal/governance/approval/registry"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	"kv-shepherd.io/shepherd/internal/notification"
	"kv-shepherd.io/shepherd/internal/observability"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
	configcodec "kv-shepherd.io/shepherd/internal/provider/configcodec"
	kubeconfigcodec "kv-shepherd.io/shepherd/internal/provider/kubeconfigcodec"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

const defaultServerInitializationTimeout = 5 * time.Second

// Compile-time check: Server must implement generated.ServerInterface.
var _ generated.ServerInterface = (*Server)(nil)

// Server implements all API handlers satisfying generated.ServerInterface.
type Server struct {
	client                   *ent.Client
	pool                     *pgxpool.Pool
	jwtCfg                   middleware.JWTConfig
	audit                    *audit.Logger
	vmService                *service.VMService
	clusterPolicy            *service.ClusterPolicyService
	approvalReqs             *service.ApprovalRequirementService
	directorySync            *service.DirectorySyncService
	vncTokens                *service.VNCTokenManager
	authSessions             *service.AuthSessionManager
	createVMUC               *usecase.CreateVMUseCase
	deleteVMUC               *usecase.DeleteVMUseCase
	externalAuth             *service.ExternalAuthService
	ticketService            *ticketing.Service
	approvalRouter           *approval.ApprovalProviderRouter // Stage 2.E: provider router
	externalApprovalRegistry *approvalregistry.Service

	authProviderConfig           *configcodec.AuthProviderConfigCodec
	kubeconfigCodec              *kubeconfigcodec.ClusterKubeconfigCodec
	publicBaseURL                string
	allowedOrigins               []string
	sessionCfg                   config.SessionConfig
	passwordPolicy               config.PasswordPolicy
	passwordHashGenerator        func(string) (string, error)
	externalAuthBeforeTokenIssue func(context.Context) error
	externalAuthFailureLog       externalAuthFailureLogFunc
	authSessionBeforeActivate    func(context.Context, string, int64) error
	loginRateLimitCfg            config.LoginRateLimit
	loginRateLimiter             *loginAttemptLimiter
	batchSubmissionGate          chan struct{}
	settingsMu                   sync.RWMutex
	externalAuthBaseURL          string
	refreshClusterHealth         func(context.Context, string) error
	riverClient                  *river.Client[pgx.Tx]
	notifier                     *notification.Triggers // Optional: notification trigger service
	traceSummaryProvider         observability.TraceSummaryProvider
	businessMetrics              observability.BusinessMetricsProvider
}

// ServerDeps holds all dependencies for creating a Server.
// ADR-0013: Manual DI, no Wire/Dig.
type ServerDeps struct {
	EntClient                *ent.Client
	Pool                     *pgxpool.Pool
	JWTCfg                   middleware.JWTConfig
	EncryptionKey            []byte
	Audit                    *audit.Logger
	VMService                *service.VMService
	ClusterPolicy            *service.ClusterPolicyService
	ApprovalReqs             *service.ApprovalRequirementService
	DirectorySync            *service.DirectorySyncService
	VNCTokens                *service.VNCTokenManager
	AuthSessions             *service.AuthSessionManager
	CreateVMUC               *usecase.CreateVMUseCase
	DeleteVMUC               *usecase.DeleteVMUseCase
	ExternalAuth             *service.ExternalAuthService
	TicketService            *ticketing.Service
	ApprovalRouter           *approval.ApprovalProviderRouter // Stage 2.E: provider router
	ExternalApprovalRegistry *approvalregistry.Service
	RefreshClusterHealth     func(context.Context, string) error
	RiverClient              *river.Client[pgx.Tx]  // ISSUE-001: needed for async VM delete/power operations
	Notifier                 *notification.Triggers // Optional: notification trigger service
	TraceSummaryProvider     observability.TraceSummaryProvider
	BusinessMetrics          observability.BusinessMetricsProvider
	PublicBaseURL            string
	AllowedOrigins           []string
	SessionConfig            config.SessionConfig
	PasswordPolicy           config.PasswordPolicy
	LoginRateLimitConfig     config.LoginRateLimit
}

// NewServer creates a new Server with all dependencies.
func NewServer(deps ServerDeps) *Server {
	authSessions := deps.AuthSessions
	if authSessions == nil {
		authSessions = service.NewAuthSessionManager(deps.Pool, deps.EntClient, deps.SessionConfig.IdleTimeout)
	}
	if authSessions != nil {
		if deps.JWTCfg.RevocationChecker == nil {
			deps.JWTCfg.RevocationChecker = authSessions
		}
		if deps.JWTCfg.ClaimsValidator == nil {
			deps.JWTCfg.ClaimsValidator = authSessions
		}
	}
	vncTokens := deps.VNCTokens
	if vncTokens == nil {
		var replay service.VNCReplayStore
		if deps.Pool != nil {
			replay = service.NewPostgresVNCReplayStore(deps.Pool)
		}
		vncTokens = service.NewVNCTokenManager(
			deps.JWTCfg.SigningKey,
			deps.EncryptionKey,
			deps.JWTCfg.Issuer,
			service.DefaultVNCTokenTTL,
			replay,
		)
	}
	externalApprovalRegistry := deps.ExternalApprovalRegistry
	if externalApprovalRegistry == nil && deps.EntClient != nil {
		externalApprovalRegistry = approvalregistry.NewService(deps.EntClient, deps.EncryptionKey)
	}
	approvalRouter := deps.ApprovalRouter
	if approvalRouter == nil && deps.TicketService != nil {
		fallback := approvalbuiltin.NewProvider(deps.TicketService)
		var activeProvider approvalcontract.ApprovalProvider = fallback
		if externalApprovalRegistry != nil {
			initCtx, cancel := serverInitializationContext()
			if provider, err := externalApprovalRegistry.ActiveProvider(initCtx, fallback); err == nil {
				activeProvider = provider
			}
			cancel()
		}
		approvalRouter = approval.NewApprovalProviderRouter(activeProvider)
	}
	srv := &Server{
		client:                   deps.EntClient,
		pool:                     deps.Pool,
		jwtCfg:                   deps.JWTCfg,
		audit:                    deps.Audit,
		vmService:                deps.VMService,
		clusterPolicy:            deps.ClusterPolicy,
		approvalReqs:             deps.ApprovalReqs,
		directorySync:            deps.DirectorySync,
		vncTokens:                vncTokens,
		authSessions:             authSessions,
		createVMUC:               deps.CreateVMUC,
		deleteVMUC:               deps.DeleteVMUC,
		externalAuth:             deps.ExternalAuth,
		ticketService:            deps.TicketService,
		approvalRouter:           approvalRouter, // Stage 2.E: provider router
		externalApprovalRegistry: externalApprovalRegistry,

		authProviderConfig:    configcodec.NewAuthProviderConfigCodec(deps.EncryptionKey),
		kubeconfigCodec:       kubeconfigcodec.NewClusterKubeconfigCodec(deps.EncryptionKey),
		publicBaseURL:         deps.PublicBaseURL,
		allowedOrigins:        append([]string(nil), deps.AllowedOrigins...),
		sessionCfg:            deps.SessionConfig,
		passwordPolicy:        deps.PasswordPolicy,
		passwordHashGenerator: HashPassword,
		loginRateLimitCfg:     normalizeLoginRateLimitConfig(deps.LoginRateLimitConfig),
		loginRateLimiter:      newLoginAttemptLimiterWithStore(deps.LoginRateLimitConfig, newPostgresLoginAttemptStore(deps.Pool)),
		batchSubmissionGate:   make(chan struct{}, 1),
		refreshClusterHealth:  deps.RefreshClusterHealth,
		riverClient:           deps.RiverClient,
		notifier:              deps.Notifier,
		traceSummaryProvider:  deps.TraceSummaryProvider,
		businessMetrics:       deps.BusinessMetrics,
	}
	initCtx, cancel := serverInitializationContext()
	srv.loadExternalAuthPlatformSetting(initCtx)
	cancel()
	return srv
}

func serverInitializationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), defaultServerInitializationTimeout)
}
