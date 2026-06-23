package service

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	"kv-shepherd.io/shepherd/ent/pendingadoption"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestAdoptionDiscoveryService_DiscoversLabelledVMs(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "adoption_discovery")
	ctx := context.Background()

	serviceEnt := seedAdoptionDiscoveryService(ctx, t, client)
	if _, err := client.VM.Create().
		SetID("vm-existing-row").
		SetName("vm-existing").
		SetInstance("01").
		SetNamespace("team-a").
		SetClusterID("cluster-a").
		SetStatus("RUNNING").
		SetCreatedBy("seed").
		SetService(serviceEnt).
		Save(ctx); err != nil {
		t.Fatalf("seed existing vm: %v", err)
	}
	if _, err := client.PendingAdoption.Create().
		SetID("pending-refresh").
		SetClusterID("cluster-a").
		SetNamespace("team-a").
		SetResourceName("vm-refresh").
		SetResourceType(pendingAdoptionVMResourceType).
		SetStatus(pendingadoption.StatusPENDING).
		SetDiscoveredBy("old-scan").
		SetLabels(map[string]string{domain.ShepherdServiceIDLabel: "svc-1", domain.ShepherdEventIDLabel: "old"}).
		Save(ctx); err != nil {
		t.Fatalf("seed pending adoption: %v", err)
	}
	if _, err := client.PendingAdoption.Create().
		SetID("pending-rejected").
		SetClusterID("cluster-a").
		SetNamespace("team-a").
		SetResourceName("vm-rejected").
		SetResourceType(pendingAdoptionVMResourceType).
		SetStatus(pendingadoption.StatusREJECTED).
		SetDiscoveredBy("admin").
		SetLabels(map[string]string{domain.ShepherdServiceIDLabel: "svc-1", domain.ShepherdEventIDLabel: "rejected-old"}).
		Save(ctx); err != nil {
		t.Fatalf("seed rejected adoption: %v", err)
	}

	infra := &namespaceProvisioningProviderStub{
		list: &domain.VMList{Items: []*domain.VM{
			{Name: "vm-adopt", Namespace: "team-a", Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel:  "svc-1",
				domain.ShepherdTemplateIDLabel: "tpl-1",
				domain.ShepherdEventIDLabel:    "evt-adopt",
			}}},
			{Name: "vm-refresh", Namespace: "team-a", Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "svc-1",
				domain.ShepherdEventIDLabel:   "evt-refresh-new",
			}}},
			{Name: "vm-existing", Namespace: "team-a", Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "svc-1",
				domain.ShepherdEventIDLabel:   "evt-existing",
			}}},
			{Name: "vm-missing-service", Namespace: "team-a", Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "missing-service",
				domain.ShepherdEventIDLabel:   "evt-missing",
			}}},
			{Name: "vm-rejected", Namespace: "team-a", Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "svc-1",
				domain.ShepherdEventIDLabel:   "evt-rejected-new",
			}}},
			{Name: "vm-unlabeled", Namespace: "team-a", Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdEventIDLabel: "evt-unlabeled",
			}}},
		}},
	}
	discovery := NewAdoptionDiscoveryService(client, NewVMService(infra))

	result, err := discovery.DiscoverVMs(ctx, AdoptionDiscoveryInput{
		ClusterID:    "cluster-a",
		Namespace:    "team-a",
		DiscoveredBy: "scanner-test",
	})
	if err != nil {
		t.Fatalf("DiscoverVMs() error = %v", err)
	}

	want := &AdoptionDiscoveryResult{
		Scanned:                6,
		Created:                1,
		Refreshed:              1,
		SkippedInvalid:         1,
		SkippedExistingVM:      1,
		SkippedMissingService:  1,
		SkippedAlreadyResolved: 1,
	}
	if diff := cmp.Diff(want, result); diff != "" {
		t.Fatalf("DiscoverVMs() result mismatch (-want +got):\n%s", diff)
	}
	if infra.lastListOpts.LabelSelector != domain.ShepherdServiceIDLabel {
		t.Fatalf("ListVMs() label selector = %q, want %q", infra.lastListOpts.LabelSelector, domain.ShepherdServiceIDLabel)
	}

	created := mustPendingAdoptionByResource(ctx, t, client, "vm-adopt")
	if created.Status != pendingadoption.StatusPENDING {
		t.Fatalf("created adoption status = %q, want PENDING", created.Status)
	}
	if created.DiscoveredBy != "scanner-test" {
		t.Fatalf("created adoption discovered_by = %q, want scanner-test", created.DiscoveredBy)
	}
	if diff := cmp.Diff(map[string]string{
		domain.ShepherdServiceIDLabel:  "svc-1",
		domain.ShepherdTemplateIDLabel: "tpl-1",
		domain.ShepherdEventIDLabel:    "evt-adopt",
	}, created.Labels); diff != "" {
		t.Fatalf("created adoption labels mismatch (-want +got):\n%s", diff)
	}

	refreshed := mustPendingAdoptionByResource(ctx, t, client, "vm-refresh")
	if refreshed.DiscoveredBy != "scanner-test" {
		t.Fatalf("refreshed adoption discovered_by = %q, want scanner-test", refreshed.DiscoveredBy)
	}
	if refreshed.Labels[domain.ShepherdEventIDLabel] != "evt-refresh-new" {
		t.Fatalf("refreshed event label = %q, want evt-refresh-new", refreshed.Labels[domain.ShepherdEventIDLabel])
	}

	rejected := mustPendingAdoptionByResource(ctx, t, client, "vm-rejected")
	if rejected.Status != pendingadoption.StatusREJECTED {
		t.Fatalf("rejected adoption status = %q, want REJECTED", rejected.Status)
	}
	if rejected.Labels[domain.ShepherdEventIDLabel] != "rejected-old" {
		t.Fatalf("rejected adoption event label = %q, want unchanged rejected-old", rejected.Labels[domain.ShepherdEventIDLabel])
	}

	for _, resourceName := range []string{"vm-existing", "vm-missing-service", "vm-unlabeled"} {
		exists, err := client.PendingAdoption.Query().
			Where(pendingadoption.ResourceNameEQ(resourceName)).
			Exist(ctx)
		require.NoError(t, err)
		if exists {
			t.Fatalf("pending adoption for %s exists, want skipped", resourceName)
		}
	}
}

func TestAdoptionDiscoveryService_DoesNotRefreshConcurrentResolvedCandidate(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "adoption_discovery_race")
	ctx := context.Background()
	seedAdoptionDiscoveryService(ctx, t, client)
	if _, err := client.PendingAdoption.Create().
		SetID("pending-refresh-race").
		SetClusterID("cluster-a").
		SetNamespace("team-a").
		SetResourceName("vm-refresh-race").
		SetResourceType(pendingAdoptionVMResourceType).
		SetStatus(pendingadoption.StatusPENDING).
		SetDiscoveredBy("old-scan").
		SetLabels(map[string]string{
			domain.ShepherdServiceIDLabel: "svc-1",
			domain.ShepherdEventIDLabel:   "old-event",
		}).
		Save(ctx); err != nil {
		t.Fatalf("seed pending adoption: %v", err)
	}
	injectPendingAdoptionStatusBeforeNextServiceUpdate(t, client, "pending-refresh-race", pendingadoption.StatusREJECTED)

	infra := &namespaceProvisioningProviderStub{
		list: &domain.VMList{Items: []*domain.VM{{
			Name:      "vm-refresh-race",
			Namespace: "team-a",
			Spec: domain.VMSpec{Labels: map[string]string{
				domain.ShepherdServiceIDLabel: "svc-1",
				domain.ShepherdEventIDLabel:   "new-event",
			}},
		}}},
	}
	discovery := NewAdoptionDiscoveryService(client, NewVMService(infra))

	result, err := discovery.DiscoverVMs(ctx, AdoptionDiscoveryInput{
		ClusterID:    "cluster-a",
		Namespace:    "team-a",
		DiscoveredBy: "scanner-test",
	})
	if err != nil {
		t.Fatalf("DiscoverVMs() error = %v", err)
	}
	want := &AdoptionDiscoveryResult{
		Scanned:                1,
		SkippedAlreadyResolved: 1,
	}
	if diff := cmp.Diff(want, result); diff != "" {
		t.Fatalf("DiscoverVMs() result mismatch (-want +got):\n%s", diff)
	}

	row, err := client.PendingAdoption.Get(ctx, "pending-refresh-race")
	require.NoError(t, err)
	if row.Status != pendingadoption.StatusREJECTED {
		t.Fatalf("pending adoption status = %s, want REJECTED", row.Status)
	}
	if row.DiscoveredBy != "old-scan" {
		t.Fatalf("pending adoption discovered_by = %q, want old-scan", row.DiscoveredBy)
	}
	if row.Labels[domain.ShepherdEventIDLabel] != "old-event" {
		t.Fatalf("pending adoption event label = %q, want old-event", row.Labels[domain.ShepherdEventIDLabel])
	}
}

func seedAdoptionDiscoveryService(ctx context.Context, t *testing.T, client *ent.Client) *ent.Service {
	t.Helper()
	system, err := client.System.Create().
		SetID("sys-1").
		SetName("shop").
		SetCreatedBy("seed").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed system: %v", err)
	}
	serviceEnt, err := client.Service.Create().
		SetID("svc-1").
		SetName("redis").
		SetSystem(system).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed service: %v", err)
	}
	return serviceEnt
}

func mustPendingAdoptionByResource(ctx context.Context, t *testing.T, client *ent.Client, resourceName string) *ent.PendingAdoption {
	t.Helper()
	row, err := client.PendingAdoption.Query().
		Where(pendingadoption.ResourceNameEQ(resourceName)).
		Only(ctx)
	if err != nil {
		t.Fatalf("query pending adoption %s: %v", resourceName, err)
	}
	return row
}

func injectPendingAdoptionStatusBeforeNextServiceUpdate(t *testing.T, client *ent.Client, id string, status pendingadoption.Status) {
	t.Helper()
	injected := false
	client.PendingAdoption.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			mutation, ok := m.(*ent.PendingAdoptionMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}
			mutationID, ok := mutation.ID()
			if !injected && ok && mutationID == id {
				injected = true
				if _, err := client.PendingAdoption.UpdateOneID(id).
					SetStatus(status).
					Save(ctx); err != nil {
					return nil, err
				}
			}
			return next.Mutate(ctx, m)
		})
	}, ent.OpUpdateOne))
}
