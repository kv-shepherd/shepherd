package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entnamespaceregistry "kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/service"
)

const (
	VMAdoptionDiscoveryScanJobKind         = "vm_adoption_discovery_scan"
	DefaultVMAdoptionDiscoveryScanInterval = 5 * time.Minute
	vmAdoptionDiscoveryScannerActor        = "system:vm-adoption-discovery"
)

// VMAdoptionDiscoveryScanArgs periodically scans healthy cluster/namespace
// pairs for Shepherd-labeled K8s VMs that are missing DB rows.
type VMAdoptionDiscoveryScanArgs struct{}

func (VMAdoptionDiscoveryScanArgs) Kind() string {
	return VMAdoptionDiscoveryScanJobKind
}

func (VMAdoptionDiscoveryScanArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: DefaultVMAdoptionDiscoveryScanInterval,
			ByQueue:  true,
			ByArgs:   true,
		},
	}
}

// VMAdoptionDiscoveryScanWorker wires the DB-scoped periodic scan around the
// service-level adoption discovery primitive.
type VMAdoptionDiscoveryScanWorker struct {
	river.WorkerDefaults[VMAdoptionDiscoveryScanArgs]
	entClient *ent.Client
	discovery *service.AdoptionDiscoveryService
}

func NewVMAdoptionDiscoveryScanWorker(entClient *ent.Client, discovery *service.AdoptionDiscoveryService) *VMAdoptionDiscoveryScanWorker {
	return &VMAdoptionDiscoveryScanWorker{
		entClient: entClient,
		discovery: discovery,
	}
}

func (w *VMAdoptionDiscoveryScanWorker) Work(ctx context.Context, _ *river.Job[VMAdoptionDiscoveryScanArgs]) error {
	if w == nil || w.entClient == nil || w.discovery == nil {
		return fmt.Errorf("vm adoption discovery scan worker is not initialized")
	}

	clusters, err := w.entClient.Cluster.Query().
		Where(
			entcluster.Enabled(true),
			entcluster.StatusEQ(entcluster.StatusHEALTHY),
		).
		Order(ent.Asc(entcluster.FieldID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list healthy clusters for adoption discovery scan: %w", err)
	}

	namespaces, err := w.entClient.NamespaceRegistry.Query().
		Where(entnamespaceregistry.Enabled(true)).
		Order(ent.Asc(entnamespaceregistry.FieldName)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list enabled namespaces for adoption discovery scan: %w", err)
	}

	var pairs, succeeded, failed int
	var scanned, created, refreshed, skippedInvalid, skippedExistingVM, skippedMissingService, skippedAlreadyResolved int
	for _, clusterRow := range clusters {
		if clusterRow == nil {
			continue
		}
		for _, namespaceRow := range namespaces {
			if namespaceRow == nil || string(namespaceRow.Environment) != string(clusterRow.Environment) {
				continue
			}
			pairs++
			result, err := w.discovery.DiscoverVMs(ctx, service.AdoptionDiscoveryInput{
				ClusterID:    clusterRow.ID,
				Namespace:    namespaceRow.Name,
				DiscoveredBy: vmAdoptionDiscoveryScannerActor,
			})
			if err != nil {
				failed++
				logger.Warn("vm adoption discovery scan failed",
					zap.String("cluster_id", clusterRow.ID),
					zap.String("namespace", namespaceRow.Name),
					zap.Error(err),
				)
				continue
			}
			succeeded++
			if result != nil {
				scanned += result.Scanned
				created += result.Created
				refreshed += result.Refreshed
				skippedInvalid += result.SkippedInvalid
				skippedExistingVM += result.SkippedExistingVM
				skippedMissingService += result.SkippedMissingService
				skippedAlreadyResolved += result.SkippedAlreadyResolved
			}
		}
	}

	logger.Info("vm adoption discovery scan completed",
		zap.Int("pairs", pairs),
		zap.Int("succeeded_pairs", succeeded),
		zap.Int("failed_pairs", failed),
		zap.Int("scanned", scanned),
		zap.Int("created", created),
		zap.Int("refreshed", refreshed),
		zap.Int("skipped_invalid", skippedInvalid),
		zap.Int("skipped_existing_vm", skippedExistingVM),
		zap.Int("skipped_missing_service", skippedMissingService),
		zap.Int("skipped_already_resolved", skippedAlreadyResolved),
	)
	return nil
}
