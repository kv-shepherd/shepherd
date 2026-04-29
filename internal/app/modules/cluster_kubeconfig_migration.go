package modules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/predicate"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	kubeconfigcodec "kv-shepherd.io/shepherd/internal/provider/kubeconfigcodec"
)

func clusterKubeconfigMigrationPredicates(cl *ent.Cluster) []predicate.Cluster {
	predicates := []predicate.Cluster{
		cluster.ID(cl.ID),
		cluster.EncryptedKubeconfigEQ(cl.EncryptedKubeconfig),
	}
	if strings.TrimSpace(cl.EncryptionKeyID) == "" {
		predicates = append(predicates, cluster.Or(
			cluster.EncryptionKeyIDIsNil(),
			cluster.EncryptionKeyIDEQ(""),
		))
		return predicates
	}
	return append(predicates, cluster.EncryptionKeyIDEQ(cl.EncryptionKeyID))
}

func migrateLegacyClusterKubeconfigs(ctx context.Context, client *ent.Client, codec *kubeconfigcodec.ClusterKubeconfigCodec) {
	if client == nil || codec == nil {
		return
	}

	clusters, err := client.Cluster.Query().All(ctx)
	if err != nil {
		logger.Warn("failed to list clusters for kubeconfig migration", zap.Error(err))
		return
	}

	migratedCount := 0
	failedCount := 0
	skippedCount := 0

	for _, cl := range clusters {
		if cl == nil || strings.TrimSpace(string(cl.EncryptedKubeconfig)) == "" {
			continue
		}

		result, err := codec.PrepareForMigration(cl.EncryptedKubeconfig, cl.EncryptionKeyID)
		if err != nil {
			failedCount++
			logger.Warn("failed to migrate legacy cluster kubeconfig",
				zap.String("cluster_id", cl.ID),
				zap.String("cluster_name", cl.Name),
				zap.Error(err),
			)
			continue
		}
		if result == nil {
			continue
		}

		updated, err := client.Cluster.Update().
			Where(clusterKubeconfigMigrationPredicates(cl)...).
			SetEncryptedKubeconfig(result.EncryptedKubeconfig).
			SetAPIServerURL(result.APIServerURL).
			SetEncryptionKeyID(result.EncryptionKeyID).
			Save(ctx)
		if err != nil {
			failedCount++
			logger.Warn("failed to persist migrated cluster kubeconfig",
				zap.String("cluster_id", cl.ID),
				zap.String("cluster_name", cl.Name),
				zap.Error(err),
			)
			continue
		}
		if updated == 0 {
			skippedCount++
			continue
		}

		migratedCount++
		logger.Info("migrated legacy cluster kubeconfig to encrypted storage",
			zap.String("cluster_id", cl.ID),
			zap.String("cluster_name", cl.Name),
		)
	}

	if migratedCount == 0 && failedCount == 0 && skippedCount == 0 {
		return
	}
	logger.Info("cluster kubeconfig migration pass completed",
		zap.Int("migrated", migratedCount),
		zap.Int("skipped", skippedCount),
		zap.Int("failed", failedCount),
	)
}

func maybeMigrateClusterKubeconfigOnRead(
	ctx context.Context,
	client *ent.Client,
	codec *kubeconfigcodec.ClusterKubeconfigCodec,
	cl *ent.Cluster,
) {
	if client == nil || codec == nil || cl == nil || strings.TrimSpace(string(cl.EncryptedKubeconfig)) == "" {
		return
	}

	result, err := codec.PrepareForMigration(cl.EncryptedKubeconfig, cl.EncryptionKeyID)
	if err != nil || result == nil {
		return
	}

	updated, updateErr := client.Cluster.Update().
		Where(clusterKubeconfigMigrationPredicates(cl)...).
		SetEncryptedKubeconfig(result.EncryptedKubeconfig).
		SetAPIServerURL(result.APIServerURL).
		SetEncryptionKeyID(result.EncryptionKeyID).
		Save(ctx)
	if updateErr != nil {
		logger.Warn("failed to persist lazy cluster kubeconfig migration",
			zap.String("cluster_id", cl.ID),
			zap.String("cluster_name", cl.Name),
			zap.Error(updateErr),
		)
		return
	}
	if updated > 0 {
		logger.Info("lazily migrated legacy cluster kubeconfig",
			zap.String("cluster_id", cl.ID),
			zap.String("cluster_name", cl.Name),
		)
	}
}

func migrateLegacyClusterKubeconfigsOnStartup(client *ent.Client, codec *kubeconfigcodec.ClusterKubeconfigCodec) {
	if client == nil || codec == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migrateLegacyClusterKubeconfigs(ctx, client, codec)
}

func loadClusterKubeconfigForRuntime(
	client *ent.Client,
	codec *kubeconfigcodec.ClusterKubeconfigCodec,
	clusterID string,
	requireHealthy bool,
) ([]byte, error) {
	if client == nil {
		return nil, fmt.Errorf("ent client is not initialized")
	}
	if codec == nil {
		return nil, fmt.Errorf("cluster kubeconfig codec is not initialized")
	}

	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, fmt.Errorf("cluster id is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cl, err := client.Cluster.Get(ctx, clusterID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("cluster %s not found", clusterID)
		}
		return nil, err
	}
	if !cl.Enabled {
		return nil, fmt.Errorf("cluster %s is disabled", clusterID)
	}
	if requireHealthy && cl.Status != cluster.StatusHEALTHY {
		return nil, fmt.Errorf("cluster %s is not healthy (status: %s)", clusterID, cl.Status)
	}
	if len(cl.EncryptedKubeconfig) == 0 {
		return nil, fmt.Errorf("cluster %s kubeconfig is empty", clusterID)
	}

	runtimeKubeconfig, err := codec.LoadForRuntime(cl.EncryptedKubeconfig, cl.EncryptionKeyID)
	if err != nil {
		return nil, err
	}
	maybeMigrateClusterKubeconfigOnRead(ctx, client, codec, cl)
	return runtimeKubeconfig, nil
}
