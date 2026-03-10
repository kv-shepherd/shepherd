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
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

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
	got := vmToAPI(vm, "prod")

	if got.Environment != generated.VMEnvironmentProd {
		t.Fatalf("environment = %q, want %q", got.Environment, generated.VMEnvironmentProd)
	}
}

func TestVmToAPI_Environment_Test(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	vm.ClusterID = "cluster-b"
	got := vmToAPI(vm, "test")

	if got.Environment != generated.VMEnvironmentTest {
		t.Fatalf("environment = %q, want %q", got.Environment, generated.VMEnvironmentTest)
	}
}

func TestVmToAPI_Environment_Empty_WhenClusterEnvBlank(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	// No clusterEnv provided → environment must remain zero value.
	got := vmToAPI(vm, "")

	if got.Environment != "" {
		t.Fatalf("environment = %q, want empty string (no cluster env)", got.Environment)
	}
}

func TestVmToAPI_PreservesCoreFields(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	vm.ClusterID = "cluster-core"
	got := vmToAPI(vm, "prod")

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
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	found := findVMInList(t, resp.Items, vmID)
	if found.Environment != generated.VMEnvironmentProd {
		t.Fatalf("vm.environment = %q, want %q", found.Environment, generated.VMEnvironmentProd)
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
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
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
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
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
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Environment != generated.VMEnvironmentProd {
		t.Fatalf("vm.environment = %q, want %q", resp.Environment, generated.VMEnvironmentProd)
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
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Environment != "" {
		t.Fatalf("vm.environment = %q, want empty (no cluster)", resp.Environment)
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

func mustCreateClusterWithEnv(t *testing.T, client *ent.Client, id string, env entcluster.Environment) {
	t.Helper()
	_, err := client.Cluster.Create().
		SetID(id).
		SetName("cl" + id[len(id)-4:]).
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
