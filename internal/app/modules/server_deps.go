package modules

import (
	"context"
	"fmt"
	"strings"
	"time"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
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
	encryptionKey := infra.EncryptionKey
	if len(encryptionKey) == 0 && cfg != nil {
		if decodedKey, err := cfg.Security.DecodeEncryptionKey(); err == nil {
			encryptionKey = decodedKey
		}
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
		EncryptionKey:        encryptionKey,
		Audit:                infra.AuditLogger,
		ExternalAuth:         service.NewExternalAuthService(infra.EntClient),
		RefreshClusterHealth: newClusterHealthRefresher(infra),
		RiverClient:          infra.RiverClient,
		PublicBaseURL:        cfg.Server.PublicBaseURL,
		AllowedOrigins:       append([]string(nil), cfg.Server.AllowedOrigins...),
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

func newClusterHealthRefresher(infra *Infrastructure) func(context.Context, string) error {
	if infra == nil || infra.EntClient == nil || infra.HealthCheck == nil {
		return nil
	}
	return func(ctx context.Context, clusterID string) error {
		clusterID = strings.TrimSpace(clusterID)
		if clusterID == "" {
			return fmt.Errorf("cluster id is required")
		}
		cl, err := infra.EntClient.Cluster.Get(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("get cluster %s: %w", clusterID, err)
		}
		if !cl.Enabled {
			infra.HealthCheck.UpdateHealth(&provider.ClusterHealth{
				ClusterName: cl.ID,
				Status:      provider.ClusterStatusUnknown,
				LastChecked: time.Now(),
				Error:       "cluster is disabled",
			})
			if _, err := infra.EntClient.Cluster.UpdateOneID(cl.ID).
				SetStatus(entcluster.StatusUNKNOWN).
				Save(ctx); err != nil {
				return fmt.Errorf("persist disabled cluster health %s: %w", cl.ID, err)
			}
			return nil
		}

		health := infra.HealthCheck.CheckCluster(ctx, cl.ID)
		infra.HealthCheck.UpdateHealth(health)

		update := infra.EntClient.Cluster.UpdateOneID(cl.ID).
			SetStatus(mapClusterHealthStatus(health.Status))
		if health.KubeVirtVersion != "" {
			update = update.SetKubevirtVersion(health.KubeVirtVersion)
		}
		if health.Status == provider.ClusterStatusHealthy {
			update = update.SetEnabledFeatures(health.EnabledFeatures)
			if health.StorageClassesDetected {
				update = update.
					SetStorageClasses(health.StorageClasses).
					SetStorageClassesUpdatedAt(health.LastChecked)
			}
		}
		if _, err := update.Save(ctx); err != nil {
			return fmt.Errorf("persist cluster health %s: %w", cl.ID, err)
		}
		return nil
	}
}

func mapClusterHealthStatus(status provider.ClusterStatus) entcluster.Status {
	switch status {
	case provider.ClusterStatusHealthy:
		return entcluster.StatusHEALTHY
	case provider.ClusterStatusUnhealthy:
		return entcluster.StatusUNHEALTHY
	case provider.ClusterStatusUnreachable:
		return entcluster.StatusUNREACHABLE
	default:
		return entcluster.StatusUNKNOWN
	}
}
