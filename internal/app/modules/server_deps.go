package modules

import (
	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
)

// NewServerDeps builds base server deps then lets each module contribute explicit wiring.
func NewServerDeps(cfg *config.Config, infra *Infrastructure, mods []Module) handlers.ServerDeps {
	deps := handlers.ServerDeps{
		EntClient:   infra.EntClient,
		Pool:        infra.Pool,
		JWTCfg:      middleware.JWTConfig{SigningKey: []byte(cfg.Security.SessionSecret), Issuer: "shepherd", ExpiresIn: cfg.Session.Lifetime},
		Audit:       infra.AuditLogger,
		RiverClient: infra.RiverClient,
	}
	for _, mod := range mods {
		if mod == nil {
			continue
		}
		mod.ContributeServerDeps(&deps)
	}
	return deps
}
