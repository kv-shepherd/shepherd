package handlers

// Tests for ListVMs and GetVM covering the cluster-environment resolution logic
// introduced per ADR-0015 §15.
//
// Coverage matrix:
//   - vmToAPI: environment populated when clusterEnv is "prod" or "test"
//   - vmToAPI: environment left empty when clusterEnv is blank
//   - ListVMs: batch-fetches cluster environments and maps each VM correctly
//   - ListVMs: VMs with no ClusterID get an empty environment
//   - ListVMs: multiple VMs spanning different environments are each resolved
//   - GetVM: single-VM path fetches cluster environment
//   - GetVM: no ClusterID → environment empty (non-fatal path)
//   - GetVM: returns 404 for unknown VM ID

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
	"kv-shepherd.io/shepherd/internal/usecase"
)

type failingVMLiveStatusProvider struct {
	*provider.MockProvider
	err error
}

func (p *failingVMLiveStatusProvider) GetVM(ctx context.Context, _, namespace, name string) (*domain.VM, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.MockProvider.GetVM(ctx, "", namespace, name)
}

func (p *failingVMLiveStatusProvider) ListVMs(ctx context.Context, _, namespace string, _ provider.ListOptions) (*domain.VMList, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.MockProvider.ListVMs(ctx, "", namespace, provider.ListOptions{})
}

type resourceExpiredOnceVMProvider struct {
	*provider.MockProvider
	calls []provider.ListOptions
}

func (p *resourceExpiredOnceVMProvider) ListVMs(ctx context.Context, _, namespace string, opts provider.ListOptions) (*domain.VMList, error) {
	p.calls = append(p.calls, opts)
	if opts.ResourceVersion == "stale-rv" {
		return nil, k8serrors.NewResourceExpired("stale resourceVersion")
	}
	return p.MockProvider.ListVMs(ctx, "", namespace, opts)
}

// ---- Pure unit tests for vmToAPI converter ------------------------------------------------

// minimalEntVM returns a minimal *ent.VM for unit tests that do not hit the DB.
func minimalEntVM(t *testing.T) *ent.VM {
	t.Helper()
	return &ent.VM{
		ID:        "vm-" + uuid.NewString(),
		Name:      "test-vm",
		Namespace: "ns-test",
		Status:    entvm.StatusRUNNING,
		Instance:  "01",
		CreatedBy: "user-a",
		CreatedAt: time.Now(),
	}
}

func TestVmToAPI_Environment_Prod(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	vm.ClusterID = "cluster-a"
	got := vmToAPI(vm, "prod", "", nil, vmSnapshotInfo{}, nil)

	if got.Environment != generated.VMEnvironmentProd {
		t.Fatalf("environment = %q, want %q", got.Environment, generated.VMEnvironmentProd)
	}
}

func TestVmToAPI_Environment_Test(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	vm.ClusterID = "cluster-b"
	got := vmToAPI(vm, "test", "", nil, vmSnapshotInfo{}, nil)

	if got.Environment != generated.VMEnvironmentTest {
		t.Fatalf("environment = %q, want %q", got.Environment, generated.VMEnvironmentTest)
	}
}

func TestVmToAPI_Environment_Empty_WhenClusterEnvBlank(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	// No clusterEnv provided → environment must remain zero value.
	got := vmToAPI(vm, "", "", nil, vmSnapshotInfo{}, nil)

	if got.Environment != "" {
		t.Fatalf("environment = %q, want empty string (no cluster env)", got.Environment)
	}
}

func TestVmToAPI_PreservesCoreFields(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	vm.ClusterID = "cluster-core"
	got := vmToAPI(vm, "prod", "", nil, vmSnapshotInfo{}, nil)

	if got.Id != vm.ID {
		t.Fatalf("id = %q, want %q", got.Id, vm.ID)
	}
	if got.Name != vm.Name {
		t.Fatalf("name = %q, want %q", got.Name, vm.Name)
	}
	if got.Namespace != vm.Namespace {
		t.Fatalf("namespace = %q, want %q", got.Namespace, vm.Namespace)
	}
	if got.ClusterId != vm.ClusterID {
		t.Fatalf("cluster_id = %q, want %q", got.ClusterId, vm.ClusterID)
	}
}

func TestVmToAPI_UsesLiveGuestOSInfoWhenAvailable(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	got := vmToAPI(
		vm,
		"",
		"",
		&domain.VM{
			OSName:    "Ubuntu 24.04.2 LTS",
			OSVersion: "24.04",
			OSFamily:  "linux",
		},
		vmSnapshotInfo{
			OSName:    "Linux",
			OSVersion: "22.04",
			OSFamily:  "linux",
		},
		nil,
	)

	if got.OsName != "Ubuntu 24.04.2 LTS" {
		t.Fatalf("os_name = %q, want live guest os name", got.OsName)
	}
	if got.OsVersion != "24.04" {
		t.Fatalf("os_version = %q, want live guest os version", got.OsVersion)
	}
	if got.OsFamily != "linux" {
		t.Fatalf("os_family = %q, want linux", got.OsFamily)
	}
}

func TestVmToAPI_FallsBackToSnapshotOSInfoWhenLiveGuestOSMissing(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	got := vmToAPI(
		vm,
		"",
		"",
		&domain.VM{},
		vmSnapshotInfo{
			OSName:    "Windows",
			OSVersion: "Server 2022",
			OSFamily:  "windows",
		},
		nil,
	)

	if got.OsName != "Windows" {
		t.Fatalf("os_name = %q, want snapshot fallback", got.OsName)
	}
	if got.OsVersion != "Server 2022" {
		t.Fatalf("os_version = %q, want snapshot fallback", got.OsVersion)
	}
	if got.OsFamily != "windows" {
		t.Fatalf("os_family = %q, want windows", got.OsFamily)
	}
}

// ---- Integration: ListVMs ----------------------------------------------------------------

func TestListVMs_PopulatesEnvironmentFromCluster(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_env_prod")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-a", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	found := findVMInList(t, resp.Items, vmID)
	if found.Environment != generated.VMEnvironmentProd {
		t.Fatalf("vm.environment = %q, want %q", found.Environment, generated.VMEnvironmentProd)
	}
	if found.ClusterName == "" {
		t.Fatal("vm.cluster_name is empty, want cluster display label fallback")
	}
}

func TestListVMs_EmptyEnvironment_WhenNoClusterID(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_no_cluster")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	vmID := "vm-" + uuid.NewString()
	svcID := mustCreateServiceForVM(t, client, "user-b")
	// Create VM with no ClusterID set.
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-nocluster" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace("test-ns").
		SetStatus(entvm.StatusPENDING).
		SetCreatedBy("user-b").
		SetServiceID(svcID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-b", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	found := findVMInList(t, resp.Items, vmID)
	if found.Environment != "" {
		t.Fatalf("vm.environment = %q, want empty (no cluster)", found.Environment)
	}
}

func TestListVMs_MultipleVMs_DifferentEnvironments(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_multi_env")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	clusterTestID := "cluster-t-" + uuid.NewString()
	clusterProdID := "cluster-p-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterTestID, entcluster.EnvironmentTest)
	mustCreateClusterWithEnv(t, client, clusterProdID, entcluster.EnvironmentProd)

	vmTestID := "vm-t-" + uuid.NewString()
	vmProdID := "vm-p-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmTestID, clusterTestID, "test-ns")
	mustCreateVMWithCluster(t, client, vmProdID, clusterProdID, "prod-ns")

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-c", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	foundTest := findVMInList(t, resp.Items, vmTestID)
	foundProd := findVMInList(t, resp.Items, vmProdID)

	if foundTest.Environment != generated.VMEnvironmentTest {
		t.Fatalf("vm-test environment = %q, want %q", foundTest.Environment, generated.VMEnvironmentTest)
	}
	if foundProd.Environment != generated.VMEnvironmentProd {
		t.Fatalf("vm-prod environment = %q, want %q", foundProd.Environment, generated.VMEnvironmentProd)
	}
}

func TestListVMs_RefreshesStatusFromLiveCluster(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_live_status")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		Save(t.Context())
	if err != nil {
		t.Fatalf("stabilize vm polling tier: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:            "vm" + vmID[len(vmID)-4:],
		Namespace:       "prod-ns",
		Cluster:         clusterID,
		Status:          domain.VMStatusStopped,
		ResourceVersion: "rv-live-1",
	}})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-live", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	found := findVMInList(t, resp.Items, vmID)
	if found.Status != generated.VMStatusSTOPPED {
		t.Fatalf("vm.status = %q, want %q", found.Status, generated.VMStatusSTOPPED)
	}

	stored, err := client.VM.Get(t.Context(), vmID)
	if err != nil {
		t.Fatalf("reload vm: %v", err)
	}
	if stored.Status != entvm.StatusSTOPPED {
		t.Fatalf("stored status = %q, want %q", stored.Status, entvm.StatusSTOPPED)
	}
}

func TestListVMs_RetriesBaselineWhenCachedResourceVersionExpires(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_live_status_rv_expired")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	vmName := "vm" + vmID[len(vmID)-4:]
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		SetLastK8sRv("stale-rv").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed stale resourceVersion: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:            vmName,
		Namespace:       "prod-ns",
		Cluster:         clusterID,
		Status:          domain.VMStatusStopped,
		ResourceVersion: "rv-live-recovered",
	}})
	expiring := &resourceExpiredOnceVMProvider{MockProvider: mock}

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(expiring),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-rv", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if len(expiring.calls) < 2 {
		t.Fatalf("ListVMs calls = %d, want at least 2", len(expiring.calls))
	}
	if got := expiring.calls[0].ResourceVersion; got != "stale-rv" {
		t.Fatalf("first resourceVersion = %q, want stale-rv", got)
	}
	if got := expiring.calls[1].ResourceVersion; got != "" {
		t.Fatalf("retry resourceVersion = %q, want empty baseline", got)
	}

	stored, err := client.VM.Get(t.Context(), vmID)
	if err != nil {
		t.Fatalf("reload vm: %v", err)
	}
	if stored.Status != entvm.StatusSTOPPED {
		t.Fatalf("stored status = %q, want %q", stored.Status, entvm.StatusSTOPPED)
	}
	if stored.LastK8sRv == nil || *stored.LastK8sRv != "rv-live-recovered" {
		t.Fatalf("stored last_k8s_rv = %v, want rv-live-recovered", stored.LastK8sRv)
	}
}

func TestListVMs_MarksUnknownWhenClusterIsUnavailable(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_unavailable_cluster")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		Save(t.Context())
	if err != nil {
		t.Fatalf("stabilize vm polling tier: %v", err)
	}

	failing := &failingVMLiveStatusProvider{
		MockProvider: provider.NewMockProvider(),
		err:          fmt.Errorf("cluster health check failed: unreachable"),
	}

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(failing),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-unavailable", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	found := findVMInList(t, resp.Items, vmID)
	if found.Status != generated.VMStatusUNKNOWN {
		t.Fatalf("vm.status = %q, want %q", found.Status, generated.VMStatusUNKNOWN)
	}
}

func TestListVMs_SupportsQuickSearchAcrossScopeAndOperatingSystem(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_search_scope_os")
	_ = logger.Init("error", "json")

	clusterAID := "cluster-a-" + uuid.NewString()
	clusterBID := "cluster-b-" + uuid.NewString()
	mustCreateClusterWithLabels(t, client, clusterAID, "prod-cluster", "Production Cluster", entcluster.EnvironmentProd)
	mustCreateClusterWithLabels(t, client, clusterBID, "test-cluster", "Test Cluster", entcluster.EnvironmentTest)

	systemA := mustCreateSystem(t, client, "sys-a-"+uuid.NewString(), "payments", "owner-a")
	systemB := mustCreateSystem(t, client, "sys-b-"+uuid.NewString(), "finance", "owner-b")
	serviceA := mustCreateService(t, client, "svc-a-"+uuid.NewString(), "billing-api", systemA.ID, "seed")
	serviceB := mustCreateService(t, client, "svc-b-"+uuid.NewString(), "ledger-api", systemB.ID, "seed")

	vmAID := "vm-a-" + uuid.NewString()
	vmBID := "vm-b-" + uuid.NewString()
	_, err := client.VM.Create().
		SetID(vmAID).
		SetName("billing-vm").
		SetInstance("01").
		SetNamespace("prod-apps").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("alice.ops").
		SetClusterID(clusterAID).
		SetServiceID(serviceA.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm A: %v", err)
	}
	_, err = client.VM.Create().
		SetID(vmBID).
		SetName("ledger-vm").
		SetInstance("01").
		SetNamespace("test-apps").
		SetStatus(entvm.StatusSTOPPED).
		SetCreatedBy("bob.ops").
		SetClusterID(clusterBID).
		SetServiceID(serviceB.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm B: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{
		{
			Name:            "billing-vm",
			Namespace:       "prod-apps",
			Cluster:         clusterAID,
			Status:          domain.VMStatusRunning,
			IPAddress:       "10.20.1.10",
			OSName:          "Ubuntu 24.04",
			OSVersion:       "24.04",
			OSFamily:        "linux",
			ResourceVersion: "rv-billing",
		},
		{
			Name:            "ledger-vm",
			Namespace:       "test-apps",
			Cluster:         clusterBID,
			Status:          domain.VMStatusStopped,
			IPAddress:       "10.20.2.20",
			OSName:          "Windows Server 2022",
			OSVersion:       "2022",
			OSFamily:        "windows",
			ResourceVersion: "rv-ledger",
		},
	})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	testCases := []struct {
		name       string
		search     string
		expectedID string
	}{
		{
			name:       "matches service name",
			search:     "billing api",
			expectedID: vmAID,
		},
		{
			name:       "matches cluster name",
			search:     "production cluster",
			expectedID: vmAID,
		},
		{
			name:       "matches operating system",
			search:     "windows",
			expectedID: vmBID,
		},
		{
			name:       "matches ip address",
			search:     "10.20.1.10",
			expectedID: vmAID,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-search", []string{"vm:read", "platform:admin"})
			srv.ListVMs(c, generated.ListVMsParams{Search: tc.search})

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
			}

			var resp generated.VMList
			if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
				t.Fatalf("decode response: %v", decodeErr)
			}
			if len(resp.Items) != 1 {
				t.Fatalf("items = %d, want 1", len(resp.Items))
			}
			if resp.Items[0].Id != tc.expectedID {
				t.Fatalf("first item id = %q, want %q", resp.Items[0].Id, tc.expectedID)
			}
		})
	}
}

func TestListVMs_SupportsExactAdvancedFilters(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_exact_filters")
	_ = logger.Init("error", "json")

	clusterAID := "cluster-a-" + uuid.NewString()
	clusterBID := "cluster-b-" + uuid.NewString()
	mustCreateClusterWithLabels(t, client, clusterAID, "prod-cluster", "Production Cluster", entcluster.EnvironmentProd)
	mustCreateClusterWithLabels(t, client, clusterBID, "test-cluster", "Test Cluster", entcluster.EnvironmentTest)

	systemA := mustCreateSystem(t, client, "sys-a-"+uuid.NewString(), "payments", "owner-a")
	systemB := mustCreateSystem(t, client, "sys-b-"+uuid.NewString(), "finance", "owner-b")
	serviceA := mustCreateService(t, client, "svc-a-"+uuid.NewString(), "billing-api", systemA.ID, "seed")
	serviceB := mustCreateService(t, client, "svc-b-"+uuid.NewString(), "ledger-api", systemB.ID, "seed")

	vmAID := "vm-a-" + uuid.NewString()
	vmBID := "vm-b-" + uuid.NewString()
	_, err := client.VM.Create().
		SetID(vmAID).
		SetName("billing-vm").
		SetInstance("01").
		SetNamespace("prod-apps").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("alice.ops").
		SetClusterID(clusterAID).
		SetServiceID(serviceA.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm A: %v", err)
	}
	_, err = client.VM.Create().
		SetID(vmBID).
		SetName("ledger-vm").
		SetInstance("01").
		SetNamespace("test-apps").
		SetStatus(entvm.StatusSTOPPED).
		SetCreatedBy("bob.ops").
		SetClusterID(clusterBID).
		SetServiceID(serviceB.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm B: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{
		{
			Name:            "billing-vm",
			Namespace:       "prod-apps",
			Cluster:         clusterAID,
			Status:          domain.VMStatusRunning,
			IPAddress:       "10.20.1.10",
			OSName:          "Ubuntu 24.04",
			OSVersion:       "24.04",
			OSFamily:        "linux",
			ResourceVersion: "rv-billing",
		},
		{
			Name:            "ledger-vm",
			Namespace:       "test-apps",
			Cluster:         clusterBID,
			Status:          domain.VMStatusStopped,
			IPAddress:       "10.20.2.20",
			OSName:          "Windows Server 2022",
			OSVersion:       "2022",
			OSFamily:        "windows",
			ResourceVersion: "rv-ledger",
		},
	})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-search", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{
		Status:    generated.ListVMsParamsStatus(entvm.StatusRUNNING),
		Namespace: "prod-apps",
		ClusterId: clusterAID,
		SystemId:  systemA.ID,
		ServiceId: serviceA.ID,
		OsName:    "Ubuntu 24.04",
		IpAddress: "10.20.1.10",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != vmAID {
		t.Fatalf("first item id = %q, want %q", resp.Items[0].Id, vmAID)
	}
}

func TestListVMs_RejectsInvalidStatus(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/vms?status=ACTIVE", "", "user-status", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{
		Status: generated.ListVMsParamsStatus("ACTIVE"),
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}

func TestListVMs_FiltersByStatusWhenRequested(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_status_filter")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	serviceID := mustCreateServiceForVM(t, client, "user-status")
	runningID := "vm-running-" + uuid.NewString()
	stoppedID := "vm-stopped-" + uuid.NewString()
	for _, item := range []struct {
		id     string
		name   string
		status entvm.Status
	}{
		{id: runningID, name: "running-vm", status: entvm.StatusRUNNING},
		{id: stoppedID, name: "stopped-vm", status: entvm.StatusSTOPPED},
	} {
		_, err := client.VM.Create().
			SetID(item.id).
			SetName(item.name).
			SetInstance("01").
			SetNamespace("status-ns").
			SetStatus(item.status).
			SetCreatedBy("user-status").
			SetServiceID(serviceID).
			Save(t.Context())
		if err != nil {
			t.Fatalf("create %s: %v", item.name, err)
		}
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms?status=RUNNING", "", "user-status", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{
		Status: generated.ListVMsParamsStatus(entvm.StatusRUNNING),
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != runningID {
		t.Fatalf("items[0].id = %q, want %q", resp.Items[0].Id, runningID)
	}
}

func TestListVMs_RejectsInvalidSortOrder(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_invalid_sort_order")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms?sort_order=sideways", "", "user-sort", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{
		SortOrder: generated.ListVMsParamsSortOrder("sideways"),
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}

func TestListVMs_RejectsInvalidSortBy(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_invalid_sort_by")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms?sort_by=owner", "", "user-sort", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{
		SortBy: "owner",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}

func TestListVMs_SortsByCreatedAtAscendingWhenRequested(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_sort_order_asc")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	serviceID := mustCreateServiceForVM(t, client, "user-sort")
	olderID := "vm-old-" + uuid.NewString()
	newerID := "vm-new-" + uuid.NewString()
	olderCreatedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	newerCreatedAt := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	_, err := client.VM.Create().
		SetID(newerID).
		SetName("newer-" + newerID[len(newerID)-6:]).
		SetInstance("01").
		SetNamespace("sort-ns").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-sort").
		SetServiceID(serviceID).
		SetCreatedAt(newerCreatedAt).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create newer vm: %v", err)
	}
	_, err = client.VM.Create().
		SetID(olderID).
		SetName("older-" + olderID[len(olderID)-6:]).
		SetInstance("01").
		SetNamespace("sort-ns").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-sort").
		SetServiceID(serviceID).
		SetCreatedAt(olderCreatedAt).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create older vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms?sort_order=asc", "", "user-sort", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{
		SortOrder: generated.ListVMsParamsSortOrderAsc,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if len(resp.Items) < 2 {
		t.Fatalf("items len = %d, want at least 2", len(resp.Items))
	}
	if resp.Items[0].Id != olderID {
		t.Fatalf("first item id = %q, want older VM %q", resp.Items[0].Id, olderID)
	}
	if resp.Items[1].Id != newerID {
		t.Fatalf("second item id = %q, want newer VM %q", resp.Items[1].Id, newerID)
	}
}

func TestListVMs_SortsByNameWhenRequested(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_sort_by_name")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	serviceID := mustCreateServiceForVM(t, client, "user-sort")
	laterByNameID := "vm-z-" + uuid.NewString()
	firstByNameID := "vm-a-" + uuid.NewString()

	_, err := client.VM.Create().
		SetID(laterByNameID).
		SetName("z-vm").
		SetInstance("01").
		SetNamespace("sort-ns").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-sort").
		SetServiceID(serviceID).
		SetCreatedAt(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create z vm: %v", err)
	}
	_, err = client.VM.Create().
		SetID(firstByNameID).
		SetName("a-vm").
		SetInstance("01").
		SetNamespace("sort-ns").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-sort").
		SetServiceID(serviceID).
		SetCreatedAt(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create a vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms?sort_by=name&sort_order=asc", "", "user-sort", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{
		SortBy:    "name",
		SortOrder: generated.ListVMsParamsSortOrderAsc,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if len(resp.Items) < 2 {
		t.Fatalf("items len = %d, want at least 2", len(resp.Items))
	}
	if resp.Items[0].Id != firstByNameID {
		t.Fatalf("first item id = %q, want VM %q", resp.Items[0].Id, firstByNameID)
	}
}

func TestGetVMFilterOptions_ReturnsReadableOptionLabels(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "vm_filter_options")
	_ = logger.Init("error", "json")

	clusterAID := "cluster-a-" + uuid.NewString()
	clusterBID := "cluster-b-" + uuid.NewString()
	mustCreateClusterWithLabels(t, client, clusterAID, "prod-cluster", "Production Cluster", entcluster.EnvironmentProd)
	mustCreateClusterWithLabels(t, client, clusterBID, "test-cluster", "Test Cluster", entcluster.EnvironmentTest)

	systemA := mustCreateSystem(t, client, "sys-a-"+uuid.NewString(), "payments", "owner-a")
	systemB := mustCreateSystem(t, client, "sys-b-"+uuid.NewString(), "finance", "owner-b")
	serviceA := mustCreateService(t, client, "svc-a-"+uuid.NewString(), "billing-api", systemA.ID, "seed")
	serviceB := mustCreateService(t, client, "svc-b-"+uuid.NewString(), "ledger-api", systemB.ID, "seed")

	_, err := client.VM.Create().
		SetID("vm-a-" + uuid.NewString()).
		SetName("billing-vm").
		SetInstance("01").
		SetNamespace("prod-apps").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("alice.ops").
		SetClusterID(clusterAID).
		SetServiceID(serviceA.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm A: %v", err)
	}
	_, err = client.VM.Create().
		SetID("vm-b-" + uuid.NewString()).
		SetName("ledger-vm").
		SetInstance("01").
		SetNamespace("test-apps").
		SetStatus(entvm.StatusSTOPPED).
		SetCreatedBy("bob.ops").
		SetClusterID(clusterBID).
		SetServiceID(serviceB.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm B: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{
		{
			Name:            "billing-vm",
			Namespace:       "prod-apps",
			Cluster:         clusterAID,
			Status:          domain.VMStatusRunning,
			IPAddress:       "10.20.1.10",
			OSName:          "Ubuntu 24.04",
			ResourceVersion: "rv-billing",
		},
		{
			Name:            "ledger-vm",
			Namespace:       "test-apps",
			Cluster:         clusterBID,
			Status:          domain.VMStatusStopped,
			IPAddress:       "10.20.2.20",
			OSName:          "Windows Server 2022",
			ResourceVersion: "rv-ledger",
		},
	})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/filter-options", "", "user-search", []string{"vm:read", "platform:admin"})
	srv.GetVMFilterOptions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMFilterOptionsResponse
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	if len(resp.Clusters) != 2 {
		t.Fatalf("clusters = %d, want 2", len(resp.Clusters))
	}
	if resp.Clusters[0].Label != "Production Cluster" {
		t.Fatalf("first cluster label = %q, want %q", resp.Clusters[0].Label, "Production Cluster")
	}
	if len(resp.Namespaces) != 2 {
		t.Fatalf("namespaces = %d, want 2", len(resp.Namespaces))
	}
	if resp.Namespaces[0].Group == "" {
		t.Fatal("expected namespace option to include environment group")
	}
	if len(resp.Services) != 2 {
		t.Fatalf("services = %d, want 2", len(resp.Services))
	}
	serviceLabels := []string{resp.Services[0].Label, resp.Services[1].Label}
	sort.Strings(serviceLabels)
	if serviceLabels[0] != "finance / ledger-api" || serviceLabels[1] != "payments / billing-api" {
		t.Fatalf("service labels = %#v, want system/service labels", resp.Services)
	}
	if len(resp.OperatingSystems) != 2 {
		t.Fatalf("operating systems = %d, want 2", len(resp.OperatingSystems))
	}
	if len(resp.IpAddresses) != 2 {
		t.Fatalf("ip addresses = %d, want 2", len(resp.IpAddresses))
	}
}

func TestCreateVMRequest_CreatesPendingCreateTicket(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "create_vm_request_success")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(provider.NewMockProvider()),
		CreateVMUC: usecase.NewCreateVMUseCase(
			client,
			service.NewVMService(provider.NewMockProvider()),
			service.NewInstanceSizeService(client),
			service.NewTemplateService(client),
		),
	})

	actor := "user-create-" + uuid.NewString()
	namespace := "team-prod-create"
	serviceID, templateID, instanceSizeID := mustCreateVMRequestPrerequisites(t, client, actor, namespace)

	body := mustJSON(t, map[string]any{
		"service_id":       serviceID,
		"template_id":      templateID,
		"instance_size_id": instanceSizeID,
		"namespace":        namespace,
		"reason":           "need capacity for release validation",
		"target_cpu_cores": 2,
		"target_memory_gi": 4,
		"target_disk_gb":   120,
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/request", body, actor, []string{"vm:create", "platform:admin"})
	srv.CreateVMRequest(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp generated.TicketResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != generated.TicketResponseStatusPENDING {
		t.Fatalf("status = %q, want %q", resp.Status, generated.TicketResponseStatusPENDING)
	}
	if resp.TicketId == "" {
		t.Fatal("ticket_id is empty")
	}

	ticket, err := client.Ticket.Get(t.Context(), resp.TicketId)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.OperationType != entticket.OperationTypeCREATE {
		t.Fatalf("ticket operation_type = %q, want %q", ticket.OperationType, entticket.OperationTypeCREATE)
	}
	if ticket.Status != entticket.StatusPENDING {
		t.Fatalf("ticket status = %q, want %q", ticket.Status, entticket.StatusPENDING)
	}
	if ticket.Requester != actor {
		t.Fatalf("ticket requester = %q, want %q", ticket.Requester, actor)
	}
	if ticket.Reason != "need capacity for release validation" {
		t.Fatalf("ticket reason = %q", ticket.Reason)
	}

	event, err := client.DomainEvent.Get(t.Context(), ticket.EventID)
	if err != nil {
		t.Fatalf("query domain event: %v", err)
	}
	if event.EventType != string(domain.EventVMCreationRequested) {
		t.Fatalf("event_type = %q, want %q", event.EventType, domain.EventVMCreationRequested)
	}
	if event.AggregateType != "vm" || event.AggregateID != serviceID {
		t.Fatalf("aggregate = %q/%q, want vm/%q", event.AggregateType, event.AggregateID, serviceID)
	}
	if event.Status != domainevent.StatusPENDING {
		t.Fatalf("event status = %q, want %q", event.Status, domainevent.StatusPENDING)
	}

	var payload domain.VMCreationPayload
	if payloadErr := json.Unmarshal(event.Payload, &payload); payloadErr != nil {
		t.Fatalf("decode event payload: %v", payloadErr)
	}
	if payload.RequesterID != actor {
		t.Fatalf("payload requester_id = %q, want %q", payload.RequesterID, actor)
	}
	if payload.ServiceID != serviceID || payload.TemplateID != templateID || payload.InstanceSizeID != instanceSizeID {
		t.Fatalf("payload ids = service:%q template:%q size:%q", payload.ServiceID, payload.TemplateID, payload.InstanceSizeID)
	}
	if payload.Namespace != namespace {
		t.Fatalf("payload namespace = %q, want %q", payload.Namespace, namespace)
	}
	if payload.TargetCPUCores != 2 || payload.TargetMemoryGi != 4 || payload.TargetDiskGB != 120 {
		t.Fatalf("payload targets = cpu:%v mem:%v disk:%d, want 2/4/120",
			payload.TargetCPUCores,
			payload.TargetMemoryGi,
			payload.TargetDiskGB,
		)
	}

	vmCount, err := client.VM.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count vms: %v", err)
	}
	if vmCount != 0 {
		t.Fatalf("vm count = %d, want 0 because create request awaits approval", vmCount)
	}
}

func TestDeleteVM_CreatesPendingDeleteTicket(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "delete_vm_request_success")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{
		EntClient:  client,
		DeleteVMUC: usecase.NewDeleteVMUseCase(client),
	})

	actor := "user-delete-" + uuid.NewString()
	fixture := mustCreateVMDeleteRequestPrerequisites(t, client, actor)

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/vms/"+fixture.VMID+"?confirm_name="+fixture.VMName,
		"",
		actor,
		[]string{"vm:delete", "platform:admin"},
	)
	srv.DeleteVM(c, fixture.VMID, generated.DeleteVMParams{ConfirmName: fixture.VMName})

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp generated.DeleteVMResponse
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Status != generated.DeleteVMResponseStatusPENDING {
		t.Fatalf("status = %q, want %q", resp.Status, generated.DeleteVMResponseStatusPENDING)
	}
	if resp.TicketId == "" || resp.EventId == "" {
		t.Fatalf("response should include ticket/event IDs: %+v", resp)
	}

	ticket, ticketErr := client.Ticket.Get(t.Context(), resp.TicketId)
	if ticketErr != nil {
		t.Fatalf("query ticket: %v", ticketErr)
	}
	if ticket.EventID != resp.EventId {
		t.Fatalf("ticket event_id = %q, want %q", ticket.EventID, resp.EventId)
	}
	if ticket.OperationType != entticket.OperationTypeDELETE {
		t.Fatalf("ticket operation_type = %q, want %q", ticket.OperationType, entticket.OperationTypeDELETE)
	}
	if ticket.Status != entticket.StatusPENDING {
		t.Fatalf("ticket status = %q, want %q", ticket.Status, entticket.StatusPENDING)
	}
	if ticket.Requester != actor {
		t.Fatalf("ticket requester = %q, want %q", ticket.Requester, actor)
	}
	if ticket.Reason != "Request to delete VM "+fixture.VMName {
		t.Fatalf("ticket reason = %q", ticket.Reason)
	}

	event, eventErr := client.DomainEvent.Get(t.Context(), resp.EventId)
	if eventErr != nil {
		t.Fatalf("query domain event: %v", eventErr)
	}
	if event.EventType != string(domain.EventVMDeletionRequested) {
		t.Fatalf("event_type = %q, want %q", event.EventType, domain.EventVMDeletionRequested)
	}
	if event.AggregateType != "vm" || event.AggregateID != fixture.VMID {
		t.Fatalf("aggregate = %q/%q, want vm/%q", event.AggregateType, event.AggregateID, fixture.VMID)
	}
	if event.Status != domainevent.StatusPENDING {
		t.Fatalf("event status = %q, want %q", event.Status, domainevent.StatusPENDING)
	}
	if event.CreatedBy != actor {
		t.Fatalf("event created_by = %q, want %q", event.CreatedBy, actor)
	}

	var payload domain.VMDeletePayload
	if payloadErr := json.Unmarshal(event.Payload, &payload); payloadErr != nil {
		t.Fatalf("decode event payload: %v", payloadErr)
	}
	if payload.VMID != fixture.VMID || payload.VMName != fixture.VMName {
		t.Fatalf("payload VM identity = %q/%q, want %q/%q", payload.VMID, payload.VMName, fixture.VMID, fixture.VMName)
	}
	if payload.Namespace != fixture.Namespace || payload.ClusterID != fixture.ClusterID || payload.ServiceID != fixture.ServiceID {
		t.Fatalf("payload scope = namespace:%q cluster:%q service:%q",
			payload.Namespace,
			payload.ClusterID,
			payload.ServiceID,
		)
	}
	if payload.Actor != actor {
		t.Fatalf("payload actor = %q, want %q", payload.Actor, actor)
	}
	if payload.RequestVMStatus != string(entvm.StatusSTOPPED) {
		t.Fatalf("payload request_vm_status = %q, want %q", payload.RequestVMStatus, entvm.StatusSTOPPED)
	}

	vmRow, vmErr := client.VM.Get(t.Context(), fixture.VMID)
	if vmErr != nil {
		t.Fatalf("reload vm: %v", vmErr)
	}
	if vmRow.Status != entvm.StatusSTOPPED {
		t.Fatalf("vm status = %q, want %q before approval", vmRow.Status, entvm.StatusSTOPPED)
	}
}

// ---- Integration: GetVM ------------------------------------------------------------------

func TestGetVM_PopulatesEnvironmentFromCluster(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_env_prod")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VM
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Environment != generated.VMEnvironmentProd {
		t.Fatalf("vm.environment = %q, want %q", resp.Environment, generated.VMEnvironmentProd)
	}
	if resp.ClusterName == "" {
		t.Fatal("vm.cluster_name is empty, want cluster display label fallback")
	}
}

func TestGetVM_EmptyEnvironment_WhenNoClusterID(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_no_cluster")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	vmID := "vm-" + uuid.NewString()
	svcID := mustCreateServiceForVM(t, client, "user-a")
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-nocluster" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace("test-ns").
		SetStatus(entvm.StatusPENDING).
		SetCreatedBy("user-a").
		SetServiceID(svcID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VM
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Environment != "" {
		t.Fatalf("vm.environment = %q, want empty (no cluster)", resp.Environment)
	}
}

func TestGetVM_RefreshesStatusFromLiveCluster(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_live_status")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		Save(t.Context())
	if err != nil {
		t.Fatalf("stabilize vm polling tier: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:            "vm" + vmID[len(vmID)-4:],
		Namespace:       "prod-ns",
		Cluster:         clusterID,
		Status:          domain.VMStatusStopped,
		ResourceVersion: "rv-live-2",
	}})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VM
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Status != generated.VMStatusSTOPPED {
		t.Fatalf("vm.status = %q, want %q", resp.Status, generated.VMStatusSTOPPED)
	}
}

func TestGetVM_DoesNotOverwriteConcurrentDeletingStatus(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_live_status_deleting_race")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	vmName := "vm" + vmID[len(vmID)-4:]
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		Save(t.Context())
	if err != nil {
		t.Fatalf("stabilize vm polling tier: %v", err)
	}
	injectVMStatusBeforeNextUpdate(t, client, vmID, entvm.StatusDELETING)

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:            vmName,
		Namespace:       "prod-ns",
		Cluster:         clusterID,
		Status:          domain.VMStatusStopped,
		ResourceVersion: "rv-live-deleting-race",
	}})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VM
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Status != generated.VMStatusDELETING {
		t.Fatalf("vm.status = %q, want %q", resp.Status, generated.VMStatusDELETING)
	}

	stored, err := client.VM.Get(t.Context(), vmID)
	if err != nil {
		t.Fatalf("reload vm: %v", err)
	}
	if stored.Status != entvm.StatusDELETING {
		t.Fatalf("stored status = %q, want %q", stored.Status, entvm.StatusDELETING)
	}
	if stored.LastK8sRv != nil {
		t.Fatalf("stored last_k8s_rv = %q, want nil", *stored.LastK8sRv)
	}
}

func TestGetVM_MarksUnknownWhenLiveVMIsMissing(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_missing_live_object")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		Save(t.Context())
	if err != nil {
		t.Fatalf("stabilize vm polling tier: %v", err)
	}

	mock := provider.NewMockProvider()
	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VM
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Status != generated.VMStatusUNKNOWN {
		t.Fatalf("vm.status = %q, want %q", resp.Status, generated.VMStatusUNKNOWN)
	}

	stored, err := client.VM.Get(t.Context(), vmID)
	if err != nil {
		t.Fatalf("reload vm: %v", err)
	}
	if stored.Status != entvm.StatusUNKNOWN {
		t.Fatalf("stored status = %q, want %q", stored.Status, entvm.StatusUNKNOWN)
	}
}

func TestGetVMRequestPrefill_ReturnsReusableCreateContext(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_request_prefill_success")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	systemID := uuid.NewString()
	serviceID := uuid.NewString()
	templateID := uuid.NewString()
	instanceSizeID := uuid.NewString()
	vmID := "vm-" + uuid.NewString()
	ticketID := "ticket-" + uuid.NewString()
	eventID := "event-" + uuid.NewString()

	mustCreateSystem(t, client, systemID, "shop-prefill", "user-a")
	mustCreateService(t, client, serviceID, "redis-prefill", systemID, "seed")

	payload, err := json.Marshal(map[string]interface{}{
		"service_id":       serviceID,
		"template_id":      templateID,
		"instance_size_id": instanceSizeID,
		"namespace":        "team-prod",
		"reason":           "scale from existing vm",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType("VM_CREATION_REQUESTED").
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payload).
		SetCreatedBy("user-a").
		Save(t.Context()); err != nil {
		t.Fatalf("create domain event: %v", err)
	}

	if _, err := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-a").
		SetReason("scale from existing vm").
		Save(t.Context()); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if _, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-prefill").
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-a").
		SetTicketID(ticketID).
		SetServiceID(serviceID).
		Save(t.Context()); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID+"/request-prefill", "", "user-a", []string{"vm:create", "platform:admin"})
	srv.GetVMRequestPrefill(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMRequestPrefill
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.SystemId.String() != systemID {
		t.Fatalf("system_id = %q, want %q", resp.SystemId.String(), systemID)
	}
	if resp.ServiceId.String() != serviceID {
		t.Fatalf("service_id = %q, want %q", resp.ServiceId.String(), serviceID)
	}
	if resp.TemplateId.String() != templateID {
		t.Fatalf("template_id = %q, want %q", resp.TemplateId.String(), templateID)
	}
	if resp.InstanceSizeId.String() != instanceSizeID {
		t.Fatalf("instance_size_id = %q, want %q", resp.InstanceSizeId.String(), instanceSizeID)
	}
	if resp.Namespace != "team-prod" {
		t.Fatalf("namespace = %q, want %q", resp.Namespace, "team-prod")
	}
	if resp.Reason != "scale from existing vm" {
		t.Fatalf("reason = %q, want %q", resp.Reason, "scale from existing vm")
	}
	if resp.BatchCount != 1 {
		t.Fatalf("batch_count = %d, want 1", resp.BatchCount)
	}
}

func TestGetVMRequestPrefill_ConflictWhenPayloadIsNotReusable(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_request_prefill_conflict")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	systemID := uuid.NewString()
	serviceID := uuid.NewString()
	vmID := "vm-" + uuid.NewString()
	ticketID := "ticket-" + uuid.NewString()
	eventID := "event-" + uuid.NewString()

	mustCreateSystem(t, client, systemID, "shop-conflict", "user-a")
	mustCreateService(t, client, serviceID, "redis-conflict", systemID, "seed")

	payload, err := json.Marshal(map[string]interface{}{
		"service_id": serviceID,
		"namespace":  "team-prod",
		"reason":     "missing template and size",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType("VM_CREATION_REQUESTED").
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payload).
		SetCreatedBy("user-a").
		Save(t.Context()); err != nil {
		t.Fatalf("create domain event: %v", err)
	}

	if _, err := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-a").
		SetReason("missing template and size").
		Save(t.Context()); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if _, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-prefill-missing-shape").
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-a").
		SetTicketID(ticketID).
		SetServiceID(serviceID).
		Save(t.Context()); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID+"/request-prefill", "", "user-a", []string{"vm:create", "platform:admin"})
	srv.GetVMRequestPrefill(c, vmID)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusConflict, w.Body.String())
	}

	var resp generated.Error
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode error response: %v", decodeErr)
	}
	if resp.Code != "VM_REQUEST_PREFILL_UNAVAILABLE" {
		t.Fatalf("error.code = %q, want %q", resp.Code, "VM_REQUEST_PREFILL_UNAVAILABLE")
	}
}

func TestGetVM_ReturnsNotFound_ForUnknownID(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_not_found")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/nonexistent-id", "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, "nonexistent-id")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VM_NOT_FOUND")
}

// ---- local seed helpers ------------------------------------------------------------------

type vmDeleteRequestFixture struct {
	VMID      string
	VMName    string
	Namespace string
	ServiceID string
	ClusterID string
}

func mustCreateVMDeleteRequestPrerequisites(t *testing.T, client *ent.Client, actor string) vmDeleteRequestFixture {
	t.Helper()

	namespace := "team-prod-del-" + uuid.NewString()[:8]
	clusterID := "cluster-" + uuid.NewString()
	serviceID := mustCreateServiceForVM(t, client, actor)
	vmID := "vm-delete-" + uuid.NewString()
	vmName := "vm-del-" + vmID[len(vmID)-4:]

	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-" + uuid.NewString()).
		SetName(namespace).
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy(actor).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	if _, err := client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace(namespace).
		SetStatus(entvm.StatusSTOPPED).
		SetCreatedBy(actor).
		SetClusterID(clusterID).
		SetServiceID(serviceID).
		Save(t.Context()); err != nil {
		t.Fatalf("create delete target vm: %v", err)
	}

	return vmDeleteRequestFixture{
		VMID:      vmID,
		VMName:    vmName,
		Namespace: namespace,
		ServiceID: serviceID,
		ClusterID: clusterID,
	}
}

func mustCreateVMRequestPrerequisites(t *testing.T, client *ent.Client, actor, namespace string) (serviceID, templateID, instanceSizeID string) {
	t.Helper()

	systemID := "sys-" + uuid.NewString()
	serviceID = uuid.NewString()
	templateID = uuid.NewString()
	instanceSizeID = uuid.NewString()

	sys := mustCreateSystem(t, client, systemID, "shop"+systemID[len(systemID)-4:], actor)
	mustCreateService(t, client, serviceID, "api"+serviceID[len(serviceID)-4:], sys.ID, "seed")

	if _, err := client.Template.Create().
		SetID(templateID).
		SetName("tpl-" + templateID[len(templateID)-4:]).
		SetSourceType(service.TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/ubuntu:24.04").
		SetCatalogScope(enttemplate.CatalogScopeProd).
		SetCreatedBy(actor).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create template: %v", err)
	}
	if _, err := client.InstanceSize.Create().
		SetID(instanceSizeID).
		SetName("size-" + instanceSizeID[len(instanceSizeID)-4:]).
		SetCPUCores(4).
		SetCPURequest(2).
		SetMemoryGi(8).
		SetMemoryRequestGi(4).
		SetDiskGB(80).
		SetCatalogScope(instancesize.CatalogScopeProd).
		SetCreatedBy(actor).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create instance size: %v", err)
	}
	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-" + uuid.NewString()).
		SetName(namespace).
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy(actor).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}

	return serviceID, templateID, instanceSizeID
}

func mustCreateClusterWithEnv(t *testing.T, client *ent.Client, id string, env entcluster.Environment) {
	t.Helper()
	mustCreateClusterWithLabels(t, client, id, "cl"+id[len(id)-4:], "cl"+id[len(id)-4:], env)
}

func mustCreateClusterWithLabels(t *testing.T, client *ent.Client, id, name, displayName string, env entcluster.Environment) {
	t.Helper()
	_, err := client.Cluster.Create().
		SetID(id).
		SetName(name).
		SetDisplayName(displayName).
		SetAPIServerURL("https://k8s.example.com").
		SetEncryptedKubeconfig([]byte("fake-kubeconfig")).
		SetCreatedBy("test-seed").
		SetEnvironment(env).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create cluster %s: %v", id, err)
	}
}

func mustCreateVMWithCluster(t *testing.T, client *ent.Client, vmID, clusterID, namespace string) {
	t.Helper()
	svcID := mustCreateServiceForVM(t, client, "user-a")
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vm" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace(namespace).
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-a").
		SetClusterID(clusterID).
		SetServiceID(svcID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm %s: %v", vmID, err)
	}
}

// mustCreateServiceForVM creates the minimal System + Service required by the VM foreign key.
func mustCreateServiceForVM(t *testing.T, client *ent.Client, actor string) string {
	t.Helper()
	sysID := "sys-" + uuid.NewString()
	svcID := "svc-" + uuid.NewString()
	_ = mustCreateSystem(t, client, sysID, "shop"+sysID[len(sysID)-4:], actor)
	mustCreateService(t, client, svcID, "redis"+svcID[len(svcID)-4:], sysID, "seed")
	return svcID
}

func injectVMStatusBeforeNextUpdate(t *testing.T, client *ent.Client, id string, status entvm.Status) {
	t.Helper()
	injected := false
	client.VM.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			mutation, ok := m.(*ent.VMMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}
			mutationID, ok := mutation.ID()
			if !injected && ok && mutationID == id {
				injected = true
				if _, err := client.VM.UpdateOneID(id).
					SetStatus(status).
					Save(ctx); err != nil {
					return nil, err
				}
			}
			return next.Mutate(ctx, m)
		})
	}, ent.OpUpdateOne))
}

// findVMInList locates a VM by ID from the list, fatally fails if not found.
func findVMInList(t *testing.T, items []generated.VM, vmID string) generated.VM {
	t.Helper()
	for i := range items {
		item := &items[i]
		if item.Id == vmID {
			return *item
		}
	}
	t.Fatalf("VM %q not found in list response", vmID)
	return generated.VM{}
}
