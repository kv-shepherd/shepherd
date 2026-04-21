package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entnamespaceregistry "kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type failingCreateProvider struct {
	*provider.MockProvider
	createErr error
}

func (p *failingCreateProvider) CreateVM(_ context.Context, clusterID, namespace string, spec *domain.VMSpec) (*domain.VM, error) {
	name := ""
	if spec != nil {
		name = spec.Name
	}
	return nil, fmt.Errorf("apply vm %s/%s on %s: %w", namespace, name, clusterID, p.createErr)
}

type vmCreateWorkerFixture struct {
	clusterID string
	eventID   string
	ticketID  string
	vmID      string
}

func seedVMCreateWorkerFixture(t *testing.T, clusterStatus cluster.Status) (*ent.Client, vmCreateWorkerFixture) {
	t.Helper()

	client := testutil.OpenEntPostgres(t, "vm_create_worker_"+strings.ToLower(clusterStatus.String())+"_"+uuid.NewString()[:8])
	ctx := t.Context()

	system, err := client.System.Create().
		SetID("sys-" + uuid.NewString()).
		SetName("sys" + uuid.NewString()[:8]).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	svc, err := client.Service.Create().
		SetID("svc-" + uuid.NewString()).
		SetName("svc" + uuid.NewString()[:8]).
		SetSystem(system).
		Save(ctx)
	require.NoError(t, err)

	clusterID := "cluster-" + uuid.NewString()
	_, err = client.Cluster.Create().
		SetID(clusterID).
		SetName("cluster-" + clusterID[len(clusterID)-4:]).
		SetAPIServerURL("https://k8s.example.com").
		SetEncryptedKubeconfig([]byte("fake-kubeconfig")).
		SetCreatedBy("seed").
		SetEnvironment(cluster.EnvironmentProd).
		SetStatus(clusterStatus).
		SetEnabled(true).
		SetDefaultStorageClass("gold-sc").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.NamespaceRegistry.Create().
		SetID("ns-" + uuid.NewString()).
		SetName("prod-ns").
		SetEnvironment(entnamespaceregistry.EnvironmentProd).
		SetEnabled(true).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	templateID := "tpl-" + uuid.NewString()
	_, err = client.Template.Create().
		SetID(templateID).
		SetName("tpl-" + uuid.NewString()[:8]).
		SetSourceType("containerdisk").
		SetImageURL("quay.io/containerdisks/ubuntu:22.04").
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	sizeID := "size-" + uuid.NewString()
	_, err = client.InstanceSize.Create().
		SetID(sizeID).
		SetName("size-" + uuid.NewString()[:8]).
		SetCPUCores(2).
		SetMemoryGi(4).
		SetDiskGB(50).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	payloadBytes, err := domain.VMCreationPayload{
		RequesterID:    "seed",
		ServiceID:      svc.ID,
		TemplateID:     templateID,
		InstanceSizeID: sizeID,
		Namespace:      "prod-ns",
	}.ToJSON()
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID(svc.ID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypeCREATE).
		SetSelectedClusterID(clusterID).
		Save(ctx)
	require.NoError(t, err)

	vmID := "vm-" + uuid.NewString()
	_, err = client.VM.Create().
		SetID(vmID).
		SetName("vm-" + uuid.NewString()[:8]).
		SetInstance("01").
		SetNamespace("prod-ns").
		SetClusterID(clusterID).
		SetStatus(entvm.StatusCREATING).
		SetCreatedBy("seed").
		SetServiceID(svc.ID).
		SetTicketID(ticketID).
		Save(ctx)
	require.NoError(t, err)

	return client, vmCreateWorkerFixture{
		clusterID: clusterID,
		eventID:   eventID,
		ticketID:  ticketID,
		vmID:      vmID,
	}
}

func TestVMCreateWorker_SnoozesWhenSelectedClusterIsUnavailable(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusUNREACHABLE)
	worker := NewVMCreateWorker(client, service.NewVMService(provider.NewMockProvider()), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Equal(t, clusterRuntimeUnavailableSnoozeDuration, snoozeErr.Duration)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusCREATING, vmRow.Status)
}

func TestVMCreateWorker_SnoozesWhenK8sCreateCannotReachCluster(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	mock := &failingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    fmt.Errorf("dial tcp 10.0.0.1:443: connect: connection refused"),
	}
	worker := NewVMCreateWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Equal(t, clusterRuntimeUnavailableSnoozeDuration, snoozeErr.Duration)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusCREATING, vmRow.Status)
}
