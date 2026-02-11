// Package handlers implements the generated ServerInterface (ADR-0021 contract-first).
//
// All methods satisfy the oapi-codegen generated ServerInterface.
// Route registration is handled by generated.RegisterHandlersWithOptions —
// handlers do NOT register their own routes.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/api/handlers
package handlers

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/notification"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

// Compile-time check: Server must implement generated.ServerInterface.
var _ generated.ServerInterface = (*Server)(nil)

// Server implements all API handlers satisfying generated.ServerInterface.
type Server struct {
	client      *ent.Client
	pool        *pgxpool.Pool
	jwtCfg      middleware.JWTConfig
	audit       *audit.Logger
	vmService   *service.VMService
	createVMUC  *usecase.CreateVMUseCase
	deleteVMUC  *usecase.DeleteVMUseCase
	gateway     *approval.Gateway
	riverClient *river.Client[pgx.Tx]
	notifier    *notification.Triggers // Optional: notification trigger service
}

// ServerDeps holds all dependencies for creating a Server.
// ADR-0013: Manual DI, no Wire/Dig.
type ServerDeps struct {
	EntClient   *ent.Client
	Pool        *pgxpool.Pool
	JWTCfg      middleware.JWTConfig
	Audit       *audit.Logger
	VMService   *service.VMService
	CreateVMUC  *usecase.CreateVMUseCase
	DeleteVMUC  *usecase.DeleteVMUseCase
	Gateway     *approval.Gateway
	RiverClient *river.Client[pgx.Tx]  // ISSUE-001: needed for async VM delete/power operations
	Notifier    *notification.Triggers // Optional: notification trigger service
}

// NewServer creates a new Server with all dependencies.
func NewServer(deps ServerDeps) *Server {
	return &Server{
		client:      deps.EntClient,
		pool:        deps.Pool,
		jwtCfg:      deps.JWTCfg,
		audit:       deps.Audit,
		vmService:   deps.VMService,
		createVMUC:  deps.CreateVMUC,
		deleteVMUC:  deps.DeleteVMUC,
		gateway:     deps.Gateway,
		riverClient: deps.RiverClient,
		notifier:    deps.Notifier,
	}
}

// actorFromCtx extracts the authenticated user ID from the request context.
// All handlers use this instead of hardcoded "anonymous".
func actorFromCtx(c interface{ GetString(string) string }) string {
	if uid := c.GetString("user_id"); uid != "" {
		return uid
	}
	return "anonymous"
}
