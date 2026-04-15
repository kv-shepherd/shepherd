package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestGetVMManifest_PlatformAdminReturnsYAML(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "vm_manifest_admin")
	clusterID := "cluster-manifest-admin"
	vmID := "vm-manifest-admin"
	namespace := "prod-ns"
	mustCreateClusterWithEnv(t, client, clusterID, "prod")
	mustCreateVMWithCluster(t, client, vmID, clusterID, namespace)

	vm, err := client.VM.Get(t.Context(), vmID)
	if err != nil {
		t.Fatalf("get vm: %v", err)
	}

	mock := provider.NewMockProvider()
	seedVMConsoleTargetInMock(mock, vm)

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID+"/manifest", "", "admin-1", []string{"platform:admin"})
	srv.GetVMManifest(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMManifestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.VmId != vmID {
		t.Fatalf("vm_id = %q, want %q", resp.VmId, vmID)
	}
	if resp.ClusterId != clusterID {
		t.Fatalf("cluster_id = %q, want %q", resp.ClusterId, clusterID)
	}
	if !strings.Contains(resp.Yaml, "kind: VirtualMachine") {
		t.Fatalf("yaml missing VM kind: %s", resp.Yaml)
	}
	if !strings.Contains(resp.Yaml, "namespace: "+namespace) {
		t.Fatalf("yaml missing namespace: %s", resp.Yaml)
	}
}

func TestGetVMManifest_RequiresPlatformAdmin(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "vm_manifest_forbidden")
	clusterID := "cluster-manifest-forbidden"
	vmID := "vm-manifest-forbidden"
	mustCreateClusterWithEnv(t, client, clusterID, "prod")
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(provider.NewMockProvider()),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID+"/manifest", "", "user-1", []string{"vm:read"})
	srv.GetVMManifest(c, vmID)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
