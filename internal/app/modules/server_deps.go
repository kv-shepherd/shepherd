package modules

import (
	"strings"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
)

// NewServerDeps builds base server deps then lets each module contribute explicit wiring.
func NewServerDeps(cfg *config.Config, infra *Infrastructure, mods []Module) handlers.ServerDeps {
	verificationKeys := make([][]byte, 0, len(cfg.Security.JWTVerificationKeys))
	for _, key := range cfg.Security.JWTVerificationKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		verificationKeys = append(verificationKeys, []byte(key))
	}

	deps := handlers.ServerDeps{
		EntClient: infra.EntClient,
		Pool:      infra.Pool,
		JWTCfg: middleware.JWTConfig{
			SigningKey:       []byte(cfg.Security.SessionSecret),
			VerificationKeys: verificationKeys,
			Issuer:           "shepherd",
			ExpiresIn:        cfg.Session.Lifetime,
		},
		Audit:       infra.AuditLogger,
		RiverClient: infra.RiverClient,
	}
	for _, mod := range mods {
		if mod == nil {
			continue
		}
		contributor, ok := mod.(ServerDepsContributor)
		if !ok {
			continue
		}
		contributor.ContributeServerDeps(&deps)
	}
	return deps
}
