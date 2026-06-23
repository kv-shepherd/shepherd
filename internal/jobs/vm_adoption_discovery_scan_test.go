package jobs

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/ent/pendingadoption"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type adoptionScanListCall struct {
	clusterID     string
	namespace     string
	labelSelector string
}

type adoptionScanProvider struct {
	*provider.MockProvider
	mu            sync.Mutex
	calls         []adoptionScanListCall
	failNamespace map[string]error
}

func (p *adoptionScanProvider) ListVMs(
	ctx context.Context,
	clusterID string,
	namespace string,
	opts provider.ListOptions,
) (*domain.VMList, error) {
	p.mu.Lock()
	p.calls = append(p.calls, adoptionScanListCall{
		clusterID:     clusterID,
		namespace:     namespace,
		labelSelector: opts.LabelSelector,
	})
	err := p.failNamespace[namespace]
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return p.MockProvider.ListVMs(ctx, clusterID, namespace, opts)
}

func (p *adoptionScanProvider) listCalls() []adoptionScanListCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]adoptionScanListCall, len(p.calls))
	copy(out, p.calls)
	return out
}

func TestVMAdoptionDiscoveryScanArgs_KindAndInsertOpts(t *testing.T) {
	var args VMAdoptionDiscoveryScanArgs
	if got := args.Kind(); got != VMAdoptionDiscoveryScanJobKind {
		t.Fatalf("Kind() = %q, want %q", got, VMAdoptionDiscoveryScanJobKind)
	}

	opts := args.InsertOpts()
	if opts.Queue != river.QueueDefault {
		t.Fatalf("InsertOpts().Queue = %q, want default", opts.Queue)
	}
	if opts.MaxAttempts != 1 {
		t.Fatalf("InsertOpts().MaxAttempts = %d, want 1", opts.MaxAttempts)
	}
	if opts.UniqueOpts.ByPeriod != DefaultVMAdoptionDiscoveryScanInterval {
		t.Fatalf("UniqueOpts.ByPeriod = %s, want %s", opts.UniqueOpts.ByPeriod, DefaultVMAdoptionDiscoveryScanInterval)
	}
	if !opts.UniqueOpts.ByQueue || !opts.UniqueOpts.ByArgs {
		t.Fatalf("UniqueOpts = %+v, want ByQueue and ByArgs", opts.UniqueOpts)
	}
}

func TestVMAdoptionDiscoveryScanWorker_WorkScansHealthyEnabledEnvironmentPairs(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "vm_adoption_discovery_scan")
	ctx := t.Context()
	seedVMAdoptionDiscoveryScanCatalog(ctx, t, client)

	infra := &adoptionScanProvider{MockProvider: provider.NewMockProvider()}
	infra.Seed([]*domain.VM{
		{
			Name:      "vm-prod-adopt",
			Namespace: "team-prod",
			Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "svc-adoption-scan",
				domain.ShepherdEventIDLabel:   "evt-prod",
			}},
		},
		{
			Name:      "vm-test-adopt",
			Namespace: "team-test",
			Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "svc-adoption-scan",
				domain.ShepherdEventIDLabel:   "evt-test",
			}},
		},
		{
			Name:      "vm-disabled-namespace",
			Namespace: "team-disabled",
			Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "svc-adoption-scan",
				domain.ShepherdEventIDLabel:   "evt-disabled",
			}},
		},
	})

	vmService := service.NewVMService(infra)
	worker := NewVMAdoptionDiscoveryScanWorker(client, service.NewAdoptionDiscoveryService(client, vmService))
	if err := worker.Work(ctx, &river.Job[VMAdoptionDiscoveryScanArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	calls := infra.listCalls()
	require.ElementsMatch(t, []adoptionScanListCall{
		{clusterID: "cluster-prod-healthy", namespace: "team-prod", labelSelector: domain.ShepherdServiceIDLabel},
		{clusterID: "cluster-test-healthy", namespace: "team-test", labelSelector: domain.ShepherdServiceIDLabel},
	}, calls)

	for _, resourceName := range []string{"vm-prod-adopt", "vm-test-adopt"} {
		row, err := client.PendingAdoption.Query().
			Where(pendingadoption.ResourceNameEQ(resourceName)).
			Only(ctx)
		require.NoError(t, err)
		if row.Status != pendingadoption.StatusPENDING {
			t.Fatalf("%s status = %s, want PENDING", resourceName, row.Status)
		}
		if row.DiscoveredBy != vmAdoptionDiscoveryScannerActor {
			t.Fatalf("%s discovered_by = %q, want %q", resourceName, row.DiscoveredBy, vmAdoptionDiscoveryScannerActor)
		}
	}

	exists, err := client.PendingAdoption.Query().
		Where(pendingadoption.ResourceNameEQ("vm-disabled-namespace")).
		Exist(ctx)
	require.NoError(t, err)
	if exists {
		t.Fatal("disabled namespace VM produced pending adoption, want skipped")
	}
}

func TestVMAdoptionDiscoveryScanWorker_WorkContinuesAfterNamespaceFailure(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "vm_adoption_discovery_scan_failure")
	ctx := t.Context()
	seedVMAdoptionDiscoveryScanCatalog(ctx, t, client)

	infra := &adoptionScanProvider{
		MockProvider:  provider.NewMockProvider(),
		failNamespace: map[string]error{"team-test": fmt.Errorf("k8s list failed")},
	}
	infra.Seed([]*domain.VM{
		{
			Name:      "vm-prod-adopt",
			Namespace: "team-prod",
			Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "svc-adoption-scan",
			}},
		},
		{
			Name:      "vm-test-adopt",
			Namespace: "team-test",
			Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "svc-adoption-scan",
			}},
		},
	})

	vmService := service.NewVMService(infra)
	worker := NewVMAdoptionDiscoveryScanWorker(client, service.NewAdoptionDiscoveryService(client, vmService))
	if err := worker.Work(ctx, &river.Job[VMAdoptionDiscoveryScanArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	prodExists, err := client.PendingAdoption.Query().
		Where(pendingadoption.ResourceNameEQ("vm-prod-adopt")).
		Exist(ctx)
	require.NoError(t, err)
	if !prodExists {
		t.Fatal("prod pending adoption missing after unrelated namespace scan failure")
	}
	testExists, err := client.PendingAdoption.Query().
		Where(pendingadoption.ResourceNameEQ("vm-test-adopt")).
		Exist(ctx)
	require.NoError(t, err)
	if testExists {
		t.Fatal("failed namespace produced pending adoption, want skipped")
	}
}

func seedVMAdoptionDiscoveryScanCatalog(ctx context.Context, t *testing.T, client *ent.Client) {
	t.Helper()
	system, err := client.System.Create().
		SetID("sys-adoption-scan").
		SetName("adsys").
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	_, err = client.Service.Create().
		SetID("svc-adoption-scan").
		SetName("adsvc").
		SetSystem(system).
		Save(ctx)
	require.NoError(t, err)

	clusters := []struct {
		id      string
		env     entcluster.Environment
		status  entcluster.Status
		enabled bool
	}{
		{id: "cluster-prod-healthy", env: entcluster.EnvironmentProd, status: entcluster.StatusHEALTHY, enabled: true},
		{id: "cluster-test-healthy", env: entcluster.EnvironmentTest, status: entcluster.StatusHEALTHY, enabled: true},
		{id: "cluster-prod-disabled", env: entcluster.EnvironmentProd, status: entcluster.StatusHEALTHY, enabled: false},
		{id: "cluster-prod-unhealthy", env: entcluster.EnvironmentProd, status: entcluster.StatusUNHEALTHY, enabled: true},
	}
	for _, clusterRow := range clusters {
		_, err := client.Cluster.Create().
			SetID(clusterRow.id).
			SetName(clusterRow.id).
			SetAPIServerURL("https://" + clusterRow.id + ".example.test").
			SetEncryptedKubeconfig([]byte("kubeconfig")).
			SetCreatedBy("seed").
			SetEnvironment(clusterRow.env).
			SetStatus(clusterRow.status).
			SetEnabled(clusterRow.enabled).
			Save(ctx)
		require.NoError(t, err)
	}

	namespaces := []struct {
		name    string
		env     namespaceregistry.Environment
		enabled bool
	}{
		{name: "team-prod", env: namespaceregistry.EnvironmentProd, enabled: true},
		{name: "team-test", env: namespaceregistry.EnvironmentTest, enabled: true},
		{name: "team-disabled", env: namespaceregistry.EnvironmentProd, enabled: false},
	}
	for _, namespaceRow := range namespaces {
		_, err := client.NamespaceRegistry.Create().
			SetID("ns-" + namespaceRow.name).
			SetName(namespaceRow.name).
			SetEnvironment(namespaceRow.env).
			SetCreatedBy("seed").
			SetEnabled(namespaceRow.enabled).
			Save(ctx)
		require.NoError(t, err)
	}
}
