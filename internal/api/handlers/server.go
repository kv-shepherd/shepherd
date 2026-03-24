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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	approvalbuiltin "kv-shepherd.io/shepherd/internal/governance/approval/builtin"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	"kv-shepherd.io/shepherd/internal/notification"
	configcodec "kv-shepherd.io/shepherd/internal/provider/configcodec"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

// Compile-time check: Server must implement generated.ServerInterface.
var _ generated.ServerInterface = (*Server)(nil)

// Server implements all API handlers satisfying generated.ServerInterface.
type Server struct {
	client               *ent.Client
	pool                 *pgxpool.Pool
	jwtCfg               middleware.JWTConfig
	audit                *audit.Logger
	vmService            *service.VMService
	clusterPolicy        *service.ClusterPolicyService
	approvalReqs         *service.ApprovalRequirementService
	directorySync        *service.DirectorySyncService
	vncTokens            *service.VNCTokenManager
	createVMUC           *usecase.CreateVMUseCase
	deleteVMUC           *usecase.DeleteVMUseCase
	externalAuth         *service.ExternalAuthService
	ticketService        *ticketing.Service
	approvalRouter       *approval.ApprovalProviderRouter // Stage 2.E: provider router
	authProviderConfig   *configcodec.AuthProviderConfigCodec
	publicBaseURL        string
	allowedOrigins       []string
	settingsMu           sync.RWMutex
	externalAuthBaseURL  string
	refreshClusterHealth func(context.Context, string) error
	riverClient          *river.Client[pgx.Tx]
	notifier             *notification.Triggers // Optional: notification trigger service
}

// ServerDeps holds all dependencies for creating a Server.
// ADR-0013: Manual DI, no Wire/Dig.
type ServerDeps struct {
	EntClient            *ent.Client
	Pool                 *pgxpool.Pool
	JWTCfg               middleware.JWTConfig
	EncryptionKey        []byte
	Audit                *audit.Logger
	VMService            *service.VMService
	ClusterPolicy        *service.ClusterPolicyService
	ApprovalReqs         *service.ApprovalRequirementService
	DirectorySync        *service.DirectorySyncService
	VNCTokens            *service.VNCTokenManager
	CreateVMUC           *usecase.CreateVMUseCase
	DeleteVMUC           *usecase.DeleteVMUseCase
	ExternalAuth         *service.ExternalAuthService
	TicketService        *ticketing.Service
	ApprovalRouter       *approval.ApprovalProviderRouter // Stage 2.E: provider router
	RefreshClusterHealth func(context.Context, string) error
	RiverClient          *river.Client[pgx.Tx]  // ISSUE-001: needed for async VM delete/power operations
	Notifier             *notification.Triggers // Optional: notification trigger service
	PublicBaseURL        string
	AllowedOrigins       []string
}

// NewServer creates a new Server with all dependencies.
func NewServer(deps ServerDeps) *Server {
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
	approvalRouter := deps.ApprovalRouter
	if approvalRouter == nil && deps.TicketService != nil {
		approvalRouter = approval.NewApprovalProviderRouter(
			approvalbuiltin.NewProvider(deps.TicketService),
		)
	}
	srv := &Server{
		client:               deps.EntClient,
		pool:                 deps.Pool,
		jwtCfg:               deps.JWTCfg,
		audit:                deps.Audit,
		vmService:            deps.VMService,
		clusterPolicy:        deps.ClusterPolicy,
		approvalReqs:         deps.ApprovalReqs,
		directorySync:        deps.DirectorySync,
		vncTokens:            vncTokens,
		createVMUC:           deps.CreateVMUC,
		deleteVMUC:           deps.DeleteVMUC,
		externalAuth:         deps.ExternalAuth,
		ticketService:        deps.TicketService,
		approvalRouter:       approvalRouter, // Stage 2.E: provider router
		authProviderConfig:   configcodec.NewAuthProviderConfigCodec(deps.EncryptionKey),
		publicBaseURL:        deps.PublicBaseURL,
		allowedOrigins:       append([]string(nil), deps.AllowedOrigins...),
		refreshClusterHealth: deps.RefreshClusterHealth,
		riverClient:          deps.RiverClient,
		notifier:             deps.Notifier,
	}
	srv.loadExternalAuthPlatformSetting(context.Background())
	return srv
}
