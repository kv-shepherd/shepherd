package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
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

type listingCreateProvider struct {
	*provider.MockProvider
	list        *domain.VMList
	listErr     error
	created     *domain.VM
	createCalls int
	createErr   error
	createdSpec *domain.VMSpec
	calls       []createListCall
}

type createListCall struct {
	cluster   string
	namespace string
	opts      provider.ListOptions
}

func (p *failingCreateProvider) CreateVM(_ context.Context, clusterID, namespace string, spec *domain.VMSpec) (*domain.VM, error) {
	name := ""
	if spec != nil {
		name = spec.Name
	}
	return nil, fmt.Errorf("apply vm %s/%s on %s: %w", namespace, name, clusterID, p.createErr)
}

func (p *listingCreateProvider) ListVMs(_ context.Context, clusterID, namespace string, opts provider.ListOptions) (*domain.VMList, error) {
	p.calls = append(p.calls, createListCall{
		cluster:   clusterID,
		namespace: namespace,
		opts:      opts,
	})
	if p.listErr != nil {
		return nil, p.listErr
	}
	return p.list, nil
}

func (p *listingCreateProvider) CreateVM(
	ctx context.Context,
	clusterID, namespace string,
	spec *domain.VMSpec,
) (*domain.VM, error) {
	p.createCalls++
	if spec != nil {
		copied := *spec
		p.createdSpec = &copied
	}
	if p.createErr != nil {
		return nil, p.createErr
	}
	if p.created != nil {
		return p.created, nil
	}
	return p.MockProvider.CreateVM(ctx, clusterID, namespace, spec)
}

type vmCreateWorkerFixture struct {
	clusterID string
	eventID   string
	sizeID    string
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
		SetCPURequest(2).
		SetMemoryGi(4).
		SetMemoryRequestGi(4).
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
		sizeID:    sizeID,
		ticketID:  ticketID,
		vmID:      vmID,
	}
}

func TestVMCreateArgs(t *testing.T) {
	t.Parallel()

	var args VMCreateArgs
	if got := args.Kind(); got != "vm_create" {
		t.Fatalf("Kind() = %q, want vm_create", got)
	}
	opts := args.InsertOpts()
	if opts.Queue != "vm_operations" {
		t.Fatalf("InsertOpts().Queue = %q, want vm_operations", opts.Queue)
	}
	if opts.MaxAttempts != 3 {
		t.Fatalf("InsertOpts().MaxAttempts = %d, want 3", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs || !opts.UniqueOpts.ByQueue {
		t.Fatalf("InsertOpts().UniqueOpts = %+v, want ByArgs and ByQueue", opts.UniqueOpts)
	}
}

func TestVMCreateWorker_FindCreatedVMByEvent(t *testing.T) {
	t.Parallel()

	eventID := "ev-" + uuid.NewString()
	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		list: &domain.VMList{Items: []*domain.VM{
			nil,
			{
				Name:      "unrelated",
				Namespace: "prod-ns",
				Spec: domain.VMSpec{
					Labels: map[string]string{"shepherd.io/event-id": "other-event"},
				},
			},
			{
				Name:      "created",
				Namespace: "prod-ns",
				Spec: domain.VMSpec{
					Labels: map[string]string{"shepherd.io/event-id": eventID},
				},
			},
		}},
	}
	worker := NewVMCreateWorker(nil, service.NewVMService(infra), nil)

	got, err := worker.findCreatedVMByEvent(t.Context(), "cluster-a", "prod-ns", eventID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "created", got.Name)
	require.Len(t, infra.calls, 1)
	require.Equal(t, "cluster-a", infra.calls[0].cluster)
	require.Equal(t, "prod-ns", infra.calls[0].namespace)
	require.Equal(t, "shepherd.io/event-id="+eventID, infra.calls[0].opts.LabelSelector)
	require.Equal(t, 1, infra.calls[0].opts.Limit)
}

func TestVMCreateWorker_FindCreatedVMByEventReturnsNilWhenNoLabelMatch(t *testing.T) {
	t.Parallel()

	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		list: &domain.VMList{Items: []*domain.VM{{
			Name:      "unrelated",
			Namespace: "prod-ns",
			Spec: domain.VMSpec{
				Labels: map[string]string{"shepherd.io/event-id": "other-event"},
			},
		}}},
	}
	worker := NewVMCreateWorker(nil, service.NewVMService(infra), nil)

	got, err := worker.findCreatedVMByEvent(t.Context(), "cluster-a", "prod-ns", "ev-"+uuid.NewString())
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestVMCreateWorker_FindCreatedVMByEventReturnsListError(t *testing.T) {
	t.Parallel()

	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		listErr:      fmt.Errorf("list api unavailable"),
	}
	worker := NewVMCreateWorker(nil, service.NewVMService(infra), nil)

	got, err := worker.findCreatedVMByEvent(t.Context(), "cluster-a", "prod-ns", "ev-"+uuid.NewString())
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "list vms")
	require.Contains(t, err.Error(), "list api unavailable")
}

func TestVMCreateWorker_SkipsK8sCreateWhenEventLabeledVMAlreadyExists(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	eventID := fixture.eventID
	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    fmt.Errorf("create should not be called"),
		list: &domain.VMList{Items: []*domain.VM{{
			Name:      "already-created",
			Namespace: "prod-ns",
			Cluster:   fixture.clusterID,
			Status:    domain.VMStatusRunning,
			Spec: domain.VMSpec{
				Labels: map[string]string{"shepherd.io/event-id": eventID},
			},
			ResourceVersion: "rv-existing-1",
		}}},
	}
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: eventID},
	})
	require.NoError(t, err)
	require.Equal(t, 0, infra.createCalls)
	require.Len(t, infra.calls, 1)
	require.Equal(t, fixture.clusterID, infra.calls[0].cluster)
	require.Equal(t, "prod-ns", infra.calls[0].namespace)
	require.Equal(t, "shepherd.io/event-id="+eventID, infra.calls[0].opts.LabelSelector)
	require.Equal(t, 1, infra.calls[0].opts.Limit)

	event, err := client.DomainEvent.Get(t.Context(), eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, vmRow.Status)
}

func TestVMCreateWorker_PreservesInstanceSizeSnapshotOvercommitRequests(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	_, err := client.Ticket.UpdateOneID(fixture.ticketID).
		SetInstanceSizeSnapshot(map[string]interface{}{
			"id":                fixture.sizeID,
			"cpu_cores":         4.0,
			"cpu_request":       2.0,
			"memory_gi":         8.0,
			"memory_request_gi": 4.0,
			"disk_gb":           50,
		}).
		Save(t.Context())
	require.NoError(t, err)

	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		list:         &domain.VMList{},
	}
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err = worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Equal(t, 1, infra.createCalls)
	require.NotNil(t, infra.createdSpec)
	require.Equal(t, 4.0, infra.createdSpec.CPU)
	require.Equal(t, 2.0, infra.createdSpec.CPURequest)
	require.Equal(t, 8.0, infra.createdSpec.MemoryGi)
	require.Equal(t, 4.0, infra.createdSpec.MemoryRequestGi)
	require.Contains(t, infra.createdSpec.RenderedYAML, `cpu: "2"`)
	require.Contains(t, infra.createdSpec.RenderedYAML, `cpu: "4"`)
	require.Contains(t, infra.createdSpec.RenderedYAML, `memory: "4Gi"`)
	require.Contains(t, infra.createdSpec.RenderedYAML, `memory: "8Gi"`)
}

func TestVMCreateWorker_NormalizesLegacySnapshotMissingRequestsToSnapshotLimits(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	_, err := client.Ticket.UpdateOneID(fixture.ticketID).
		SetInstanceSizeSnapshot(map[string]interface{}{
			"id":        fixture.sizeID,
			"cpu_cores": 4.0,
			"memory_gi": 8.0,
			"disk_gb":   50,
		}).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.InstanceSize.UpdateOneID(fixture.sizeID).
		SetCPUCores(8.0).
		SetCPURequest(6.0).
		SetMemoryGi(16.0).
		SetMemoryRequestGi(12.0).
		Save(t.Context())
	require.NoError(t, err)

	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		list:         &domain.VMList{},
	}
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err = worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Equal(t, 1, infra.createCalls)
	require.NotNil(t, infra.createdSpec)
	require.Equal(t, 4.0, infra.createdSpec.CPU)
	require.Equal(t, 4.0, infra.createdSpec.CPURequest)
	require.Equal(t, 8.0, infra.createdSpec.MemoryGi)
	require.Equal(t, 8.0, infra.createdSpec.MemoryRequestGi)
}

func TestVMCreateWorker_DoesNotOverwriteConcurrentDeletingStatusOnCompletedPersistence(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	eventID := fixture.eventID
	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    fmt.Errorf("create should not be called"),
		list: &domain.VMList{Items: []*domain.VM{{
			Name:      "already-created",
			Namespace: "prod-ns",
			Cluster:   fixture.clusterID,
			Status:    domain.VMStatusRunning,
			Spec: domain.VMSpec{
				Labels: map[string]string{"shepherd.io/event-id": eventID},
			},
			ResourceVersion: "rv-existing-1",
		}}},
	}
	injectVMStatusBeforeNextJobsUpdate(t, client, fixture.vmID, entvm.StatusDELETING)
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: eventID},
	})
	require.NoError(t, err)
	require.Equal(t, 0, infra.createCalls)

	event, err := client.DomainEvent.Get(t.Context(), eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, vmRow.Status)
}

func TestVMCreateWorker_CompletedEventRepairsTicketSuccessWithoutCreating(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(t.Context())
	require.NoError(t, err)
	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    fmt.Errorf("create should not be called"),
		listErr:      fmt.Errorf("list should not be called"),
	}
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err = worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Empty(t, infra.calls)
	require.Equal(t, 0, infra.createCalls)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestVMCreateWorker_SkipsFailedEventWithoutCreating(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(t.Context())
	require.NoError(t, err)

	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    fmt.Errorf("create should not be called"),
		listErr:      fmt.Errorf("list should not be called"),
	}
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err = worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Empty(t, infra.calls)
	require.Equal(t, 0, infra.createCalls)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusCREATING, vmRow.Status)
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
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   VMCreateArgs{EventID: fixture.eventID},
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

func TestVMCreateWorker_RetryableCreateFailureDoesNotPersistTerminalFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	mock := &failingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    errors.New("admission webhook temporarily rejected create"),
	}
	worker := NewVMCreateWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execute k8s create")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "retryable create failure must not cancel the job")
	var snoozeErr *river.JobSnoozeError
	require.False(t, errors.As(err, &snoozeErr), "ordinary create failure should remain a retryable error")

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

func TestVMCreateWorker_FinalCreateFailurePersistsTerminalFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	mock := &failingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    errors.New("admission webhook rejected final create attempt"),
	}
	worker := NewVMCreateWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMCreateArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execute k8s create")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "final provider failure should let River finalize the ordinary error")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusFAILED, vmRow.Status)
}

func TestVMCreateWorker_FinalCreateFailureDoesNotOverwriteConcurrentDeletingStatus(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	mock := &failingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    errors.New("admission webhook rejected final create attempt"),
	}
	injectVMStatusBeforeNextJobsUpdate(t, client, fixture.vmID, entvm.StatusDELETING)
	worker := NewVMCreateWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMCreateArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execute k8s create")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, vmRow.Status)
}

func TestVMCreateWorker_FinalCreateFailureRollsBackVMOnEventPersistFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	client.DomainEvent.Use(enthook.On(
		failDomainEventStatusUpdateHook(domainevent.StatusFAILED, errors.New("failed final create event persist unavailable")),
		ent.OpUpdate,
	))
	mock := &failingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    errors.New("admission webhook rejected final create attempt"),
	}
	worker := NewVMCreateWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMCreateArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist final FAILED status for create event")
	require.Contains(t, err.Error(), "failed final create event persist unavailable")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusCREATING, vmRow.Status)
}

func TestVMCreateWorker_ContextCancellationDoesNotFailEventTicketOrVM(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	mock := &failingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		createErr:    context.Canceled,
	}
	worker := NewVMCreateWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.ErrorIs(t, err, context.Canceled)

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

func TestVMCreateWorker_VMStatusPersistCancellationReturnsContextError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		list:         &domain.VMList{},
		created: &domain.VM{
			Name:      "created-vm-status-canceled",
			Namespace: "prod-ns",
			Cluster:   fixture.clusterID,
			Status:    domain.VMStatusRunning,
		},
	}
	client.VM.Use(enthook.On(
		enthook.FixedError(errors.Join(errors.New("vm status persist interrupted"), context.Canceled)),
		ent.OpUpdateOne,
	))
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.Equal(t, context.Canceled, err)
	require.Equal(t, 1, infra.createCalls)

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

func TestVMCreateWorker_EventCompletePersistCancellationReturnsContextError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		list:         &domain.VMList{},
		created: &domain.VM{
			Name:      "created-event-complete-canceled",
			Namespace: "prod-ns",
			Cluster:   fixture.clusterID,
			Status:    domain.VMStatusRunning,
		},
	}
	client.DomainEvent.Use(enthook.On(
		failDomainEventStatusUpdateHook(domainevent.StatusCOMPLETED, errors.Join(errors.New("event complete persist interrupted"), context.Canceled)),
		ent.OpUpdate,
	))
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.Equal(t, context.Canceled, err)
	require.Equal(t, 1, infra.createCalls)

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

func TestVMCreateWorker_EventCompletePersistFailureRollsBackVMAndTicket(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		list:         &domain.VMList{},
		created: &domain.VM{
			Name:      "created-event-complete-failed",
			Namespace: "prod-ns",
			Cluster:   fixture.clusterID,
			Status:    domain.VMStatusRunning,
		},
	}
	client.DomainEvent.Use(enthook.On(
		failDomainEventStatusUpdateHook(domainevent.StatusCOMPLETED, errors.New("event complete persist failed")),
		ent.OpUpdate,
	))
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist completed create state for event")
	require.Contains(t, err.Error(), "event complete persist failed")
	require.Equal(t, 1, infra.createCalls)

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

func TestVMCreateWorker_TicketSuccessPersistFailureReturnsRetryableError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture := seedVMCreateWorkerFixture(t, cluster.StatusHEALTHY)
	infra := &listingCreateProvider{
		MockProvider: provider.NewMockProvider(),
		list:         &domain.VMList{},
		created: &domain.VM{
			Name:      "created-ticket-success-failed",
			Namespace: "prod-ns",
			Cluster:   fixture.clusterID,
			Status:    domain.VMStatusRunning,
		},
	}
	updateCount := 0
	client.Ticket.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			updateCount++
			if updateCount == 2 {
				return nil, errors.New("ticket success persist failed")
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdate))
	worker := NewVMCreateWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist completed create state for event")
	require.Contains(t, err.Error(), "ticket success persist failed")
	require.Equal(t, 1, infra.createCalls)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusCREATING, vmRow.Status)
}

func TestVMCreateWorker_MalformedPayloadFailurePersistErrorIsRetryable(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client := testutil.OpenEntPostgres(t, "vm_create_malformed_failed_persist_"+uuid.NewString()[:8])
	eventID := "ev-" + uuid.NewString()
	_, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(t.Context())
	require.NoError(t, err)
	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypeCREATE).
		Save(t.Context())
	require.NoError(t, err)
	client.DomainEvent.Use(enthook.On(
		failDomainEventStatusUpdateHook(domainevent.StatusFAILED, errors.New("failed event persist unavailable")),
		ent.OpUpdate,
	))
	worker := NewVMCreateWorker(client, service.NewVMService(provider.NewMockProvider()), nil)

	err = worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist FAILED status for malformed create event")
	require.Contains(t, err.Error(), "failed event persist unavailable")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr))

	event, err := client.DomainEvent.Get(t.Context(), eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
}

func TestVMCreateWorker_PermanentValidationFailurePersistErrorIsRetryable(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client := testutil.OpenEntPostgres(t, "vm_create_validation_failed_persist_"+uuid.NewString()[:8])
	payloadBytes, err := domain.VMCreationPayload{
		RequesterID:    "seed",
		ServiceID:      "svc-" + uuid.NewString(),
		TemplateID:     "tpl-" + uuid.NewString(),
		InstanceSizeID: "size-" + uuid.NewString(),
		Namespace:      "",
	}.ToJSON()
	require.NoError(t, err)
	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(t.Context())
	require.NoError(t, err)
	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypeCREATE).
		Save(t.Context())
	require.NoError(t, err)
	client.DomainEvent.Use(enthook.On(
		failDomainEventStatusUpdateHook(domainevent.StatusFAILED, errors.New("validation failure state persist unavailable")),
		ent.OpUpdate,
	))
	worker := NewVMCreateWorker(client, service.NewVMService(provider.NewMockProvider()), nil)

	err = worker.Work(t.Context(), &river.Job[VMCreateArgs]{
		Args: VMCreateArgs{EventID: eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist FAILED status for create event")
	require.Contains(t, err.Error(), "validation failure state persist unavailable")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr))

	event, err := client.DomainEvent.Get(t.Context(), eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
}
