package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
)

func init() {
	_ = logger.Init("error", "json")
}

type namespaceProvisioningProviderStub struct {
	ensureCalls   int
	validateCalls int
	createCalls   int
	getCalls      int
	listCalls     int
	updateCalls   int
	deleteCalls   int
	startCalls    int
	stopCalls     int
	restartCalls  int
	manifestCalls int
	dryRunCalls   int
	mutationCalls int
	lastCluster   string
	lastNamespace string
	lastName      string
	lastGetName   string
	lastListOpts  provider.ListOptions
	lastMutation  *domain.VMMutation
	ensureErr     error
	createErr     error
	getVM         *domain.VM
	getErr        error
	list          *domain.VMList
	listErr       error
	updateVM      *domain.VM
	updateErr     error
	deleteErr     error
	startErr      error
	stopErr       error
	restartErr    error
	manifestYAML  string
	manifestErr   error
	dryRunErr     error
	mutationVM    *domain.VM
	mutationErr   error
	vncConn       net.Conn
	vncErr        error
	serialConn    net.Conn
	serialErr     error
}

func (s *namespaceProvisioningProviderStub) Name() string { return "stub" }
func (s *namespaceProvisioningProviderStub) Type() string { return "stub" }

func (s *namespaceProvisioningProviderStub) EnsureNamespace(_ context.Context, cluster, namespace string) error {
	s.ensureCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	return s.ensureErr
}

func (s *namespaceProvisioningProviderStub) GetVM(_ context.Context, cluster, namespace, name string) (*domain.VM, error) {
	s.getCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastGetName = name
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getVM != nil {
		return s.getVM, nil
	}
	return nil, fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) ListVMs(_ context.Context, cluster, namespace string, opts provider.ListOptions) (*domain.VMList, error) {
	s.listCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastListOpts = opts
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.list != nil {
		return s.list, nil
	}
	return &domain.VMList{}, nil
}

func (s *namespaceProvisioningProviderStub) CreateVM(_ context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.VM, error) {
	s.createCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &domain.VM{Name: spec.Name, Namespace: namespace, Spec: *spec}, nil
}

func (s *namespaceProvisioningProviderStub) UpdateVM(_ context.Context, cluster, namespace, name string, spec *domain.VMSpec) (*domain.VM, error) {
	s.updateCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastName = name
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	if s.updateVM != nil {
		return s.updateVM, nil
	}
	return &domain.VM{Name: name, Namespace: namespace, Spec: *spec}, nil
}

func (s *namespaceProvisioningProviderStub) DeleteVM(_ context.Context, cluster, namespace, name string) error {
	s.deleteCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastName = name
	return s.deleteErr
}

func (s *namespaceProvisioningProviderStub) StartVM(_ context.Context, cluster, namespace, name string) error {
	s.startCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastName = name
	return s.startErr
}

func (s *namespaceProvisioningProviderStub) StopVM(_ context.Context, cluster, namespace, name string) error {
	s.stopCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastName = name
	return s.stopErr
}

func (s *namespaceProvisioningProviderStub) RestartVM(_ context.Context, cluster, namespace, name string) error {
	s.restartCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastName = name
	return s.restartErr
}

func (s *namespaceProvisioningProviderStub) PauseVM(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) UnpauseVM(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}

func (s *namespaceProvisioningProviderStub) ValidateSpec(_ context.Context, _, _ string, _ *domain.VMSpec) (*domain.ValidationResult, error) {
	s.validateCalls++
	return &domain.ValidationResult{Valid: true}, nil
}

func (s *namespaceProvisioningProviderStub) GetVMManifestYAML(_ context.Context, cluster, namespace, name string) (string, error) {
	s.manifestCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastName = name
	if s.manifestErr != nil {
		return "", s.manifestErr
	}
	return s.manifestYAML, nil
}

func (s *namespaceProvisioningProviderStub) DryRunVMMutation(_ context.Context, cluster, namespace, name string, mutation *domain.VMMutation) error {
	s.dryRunCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastName = name
	s.lastMutation = mutation
	return s.dryRunErr
}

func (s *namespaceProvisioningProviderStub) ExecuteVMMutation(_ context.Context, cluster, namespace, name string, mutation *domain.VMMutation) (*domain.VM, error) {
	s.mutationCalls++
	s.lastCluster = cluster
	s.lastNamespace = namespace
	s.lastName = name
	s.lastMutation = mutation
	if s.mutationErr != nil {
		return nil, s.mutationErr
	}
	if s.mutationVM != nil {
		return s.mutationVM, nil
	}
	return &domain.VM{Name: name, Namespace: namespace}, nil
}

func (s *namespaceProvisioningProviderStub) OpenVNCStream(_ context.Context, _, _, _ string) (net.Conn, error) {
	if s.vncErr != nil {
		return nil, s.vncErr
	}
	return s.vncConn, nil
}

func (s *namespaceProvisioningProviderStub) OpenSerialConsoleStream(_ context.Context, _, _, _ string) (net.Conn, error) {
	if s.serialErr != nil {
		return nil, s.serialErr
	}
	return s.serialConn, nil
}

func TestVMServiceValidateAndPrepare_EnsuresNamespaceFirst(t *testing.T) {
	t.Parallel()

	infra := &namespaceProvisioningProviderStub{}
	svc := NewVMService(infra)

	spec := &domain.VMSpec{
		Name:            "vm-a",
		CPU:             2,
		CPURequest:      2,
		MemoryGi:        4,
		MemoryRequestGi: 4,
		DiskGB:          50,
		Image:           "docker://quay.io/containerdisks/ubuntu:22.04",
	}

	result, err := svc.ValidateAndPrepare(t.Context(), "cluster-a", "team-a", spec)
	if err != nil {
		t.Fatalf("ValidateAndPrepare() error = %v", err)
	}
	if result == nil || !result.Valid {
		t.Fatalf("ValidateAndPrepare() result = %#v, want valid", result)
	}
	if infra.ensureCalls != 1 {
		t.Fatalf("EnsureNamespace calls = %d, want 1", infra.ensureCalls)
	}
	if infra.validateCalls != 1 {
		t.Fatalf("ValidateSpec calls = %d, want 1", infra.validateCalls)
	}
	if infra.lastCluster != "cluster-a" || infra.lastNamespace != "team-a" {
		t.Fatalf("EnsureNamespace args = (%q,%q), want (%q,%q)", infra.lastCluster, infra.lastNamespace, "cluster-a", "team-a")
	}
}

func TestVMServiceExecuteK8sCreate_EnsuresNamespaceFirst(t *testing.T) {
	t.Parallel()

	infra := &namespaceProvisioningProviderStub{}
	svc := NewVMService(infra)

	spec := &domain.VMSpec{
		Name:            "vm-b",
		CPU:             2,
		CPURequest:      2,
		MemoryGi:        4,
		MemoryRequestGi: 4,
		DiskGB:          50,
		Image:           "docker://quay.io/containerdisks/ubuntu:22.04",
	}

	vm, err := svc.ExecuteK8sCreate(t.Context(), "cluster-b", "team-b", spec)
	if err != nil {
		t.Fatalf("ExecuteK8sCreate() error = %v", err)
	}
	if vm == nil || vm.Namespace != "team-b" {
		t.Fatalf("ExecuteK8sCreate() vm = %#v, want namespace team-b", vm)
	}
	if infra.ensureCalls != 1 {
		t.Fatalf("EnsureNamespace calls = %d, want 1", infra.ensureCalls)
	}
	if infra.createCalls != 1 {
		t.Fatalf("CreateVM calls = %d, want 1", infra.createCalls)
	}
}

func TestVMServiceExecuteK8sCreate_ReturnsExistingVMOnMatchingAlreadyExists(t *testing.T) {
	t.Parallel()

	eventID := "ev-create-1"
	infra := &namespaceProvisioningProviderStub{
		createErr: alreadyExistsVMError("vm-existing"),
		getVM: &domain.VM{
			Name:      "vm-existing",
			Namespace: "team-existing",
			Status:    domain.VMStatusRunning,
			Spec: domain.VMSpec{
				Labels: map[string]string{domain.ShepherdEventIDLabel: eventID},
			},
		},
	}
	svc := NewVMService(infra)

	vm, err := svc.ExecuteK8sCreate(t.Context(), "cluster-existing", "team-existing", &domain.VMSpec{
		Name:            "vm-existing",
		CPU:             2,
		CPURequest:      2,
		MemoryGi:        4,
		MemoryRequestGi: 4,
		DiskGB:          50,
		Image:           "docker://quay.io/containerdisks/ubuntu:22.04",
		Labels:          map[string]string{domain.ShepherdEventIDLabel: eventID},
	})
	if err != nil {
		t.Fatalf("ExecuteK8sCreate() error = %v", err)
	}
	if vm == nil || vm.Name != "vm-existing" || vm.Status != domain.VMStatusRunning {
		t.Fatalf("ExecuteK8sCreate() vm = %#v, want existing running VM", vm)
	}
	if infra.ensureCalls != 1 || infra.createCalls != 1 || infra.getCalls != 1 {
		t.Fatalf(
			"calls = ensure:%d create:%d get:%d, want 1 each",
			infra.ensureCalls,
			infra.createCalls,
			infra.getCalls,
		)
	}
	if infra.lastGetName != "vm-existing" {
		t.Fatalf("GetVM name = %q, want vm-existing", infra.lastGetName)
	}
}

func TestVMServiceExecuteK8sCreate_RejectsAlreadyExistsWithDifferentEventLabel(t *testing.T) {
	t.Parallel()

	infra := &namespaceProvisioningProviderStub{
		createErr: alreadyExistsVMError("vm-existing"),
		getVM: &domain.VM{
			Name:      "vm-existing",
			Namespace: "team-existing",
			Spec: domain.VMSpec{
				Labels: map[string]string{domain.ShepherdEventIDLabel: "ev-other"},
			},
		},
	}
	svc := NewVMService(infra)

	_, err := svc.ExecuteK8sCreate(t.Context(), "cluster-existing", "team-existing", &domain.VMSpec{
		Name:            "vm-existing",
		CPU:             2,
		CPURequest:      2,
		MemoryGi:        4,
		MemoryRequestGi: 4,
		DiskGB:          50,
		Image:           "docker://quay.io/containerdisks/ubuntu:22.04",
		Labels:          map[string]string{domain.ShepherdEventIDLabel: "ev-requested"},
	})
	if err == nil {
		t.Fatal("ExecuteK8sCreate() expected ownership mismatch error, got nil")
	}
	if !apierrors.IsAlreadyExists(err) {
		t.Fatalf("ExecuteK8sCreate() error = %v, want wrapped AlreadyExists", err)
	}
	if got := err.Error(); !containsAll(got, "verify existing vm", "does not match requested") {
		t.Fatalf("ExecuteK8sCreate() error = %q, want ownership mismatch context", got)
	}
	if infra.createCalls != 1 || infra.getCalls != 1 {
		t.Fatalf("calls = create:%d get:%d, want 1 each", infra.createCalls, infra.getCalls)
	}
}

func TestVMServiceExecuteK8sCreate_ReturnsAlreadyExistsWhenExistingVMLookupFails(t *testing.T) {
	t.Parallel()

	infra := &namespaceProvisioningProviderStub{
		createErr: alreadyExistsVMError("vm-existing"),
		getErr:    fmt.Errorf("api temporarily unavailable"),
	}
	svc := NewVMService(infra)

	_, err := svc.ExecuteK8sCreate(t.Context(), "cluster-existing", "team-existing", &domain.VMSpec{
		Name:            "vm-existing",
		CPU:             2,
		CPURequest:      2,
		MemoryGi:        4,
		MemoryRequestGi: 4,
		DiskGB:          50,
		Image:           "docker://quay.io/containerdisks/ubuntu:22.04",
		Labels:          map[string]string{domain.ShepherdEventIDLabel: "ev-requested"},
	})
	if err == nil {
		t.Fatal("ExecuteK8sCreate() expected lookup error, got nil")
	}
	if !apierrors.IsAlreadyExists(err) {
		t.Fatalf("ExecuteK8sCreate() error = %v, want wrapped AlreadyExists", err)
	}
	if got := err.Error(); !containsAll(got, "verify existing vm", "api temporarily unavailable") {
		t.Fatalf("ExecuteK8sCreate() error = %q, want lookup failure context", got)
	}
	if infra.createCalls != 1 || infra.getCalls != 1 {
		t.Fatalf("calls = create:%d get:%d, want 1 each", infra.createCalls, infra.getCalls)
	}
}

func TestVMServiceProviderReadMethods_DelegateAndWrapErrors(t *testing.T) {
	t.Parallel()

	t.Run("GetVM", func(t *testing.T) {
		t.Parallel()
		wantVM := &domain.VM{Name: "vm-read", Namespace: "team-read"}
		infra := &namespaceProvisioningProviderStub{getVM: wantVM}
		svc := NewVMService(infra)

		got, err := svc.GetVM(t.Context(), "cluster-read", "team-read", "vm-read")
		if err != nil {
			t.Fatalf("GetVM() error = %v", err)
		}
		if got != wantVM {
			t.Fatalf("GetVM() = %#v, want fixture VM", got)
		}
		if infra.getCalls != 1 || infra.lastCluster != "cluster-read" ||
			infra.lastNamespace != "team-read" || infra.lastGetName != "vm-read" {
			t.Fatalf(
				"GetVM calls/args = calls:%d cluster:%q namespace:%q name:%q",
				infra.getCalls,
				infra.lastCluster,
				infra.lastNamespace,
				infra.lastGetName,
			)
		}
	})

	t.Run("GetVM wraps provider error", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{getErr: fmt.Errorf("api unavailable")}
		svc := NewVMService(infra)

		_, err := svc.GetVM(t.Context(), "cluster-read", "team-read", "vm-read")
		if err == nil {
			t.Fatal("GetVM() expected provider error, got nil")
		}
		if got := err.Error(); !containsAll(got, "get vm", "api unavailable") {
			t.Fatalf("GetVM() error = %q, want wrapped provider context", got)
		}
	})

	t.Run("ListVMs", func(t *testing.T) {
		t.Parallel()
		wantList := &domain.VMList{Items: []*domain.VM{{Name: "vm-list"}}, TotalCount: 1}
		infra := &namespaceProvisioningProviderStub{list: wantList}
		svc := NewVMService(infra)

		opts := provider.ListOptions{LabelSelector: "app=demo", Limit: 25, SkipVMIEnrichment: true}
		got, err := svc.ListVMs(t.Context(), "cluster-list", "team-list", opts)
		if err != nil {
			t.Fatalf("ListVMs() error = %v", err)
		}
		if got != wantList {
			t.Fatalf("ListVMs() = %#v, want fixture list", got)
		}
		if infra.listCalls != 1 || infra.lastCluster != "cluster-list" ||
			infra.lastNamespace != "team-list" || infra.lastListOpts != opts {
			t.Fatalf(
				"ListVMs calls/args = calls:%d cluster:%q namespace:%q opts:%+v",
				infra.listCalls,
				infra.lastCluster,
				infra.lastNamespace,
				infra.lastListOpts,
			)
		}
	})

	t.Run("ListVMs wraps provider error", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{listErr: fmt.Errorf("list failed")}
		svc := NewVMService(infra)

		_, err := svc.ListVMs(t.Context(), "cluster-list", "team-list", provider.ListOptions{})
		if err == nil {
			t.Fatal("ListVMs() expected provider error, got nil")
		}
		if got := err.Error(); !containsAll(got, "list vms", "list failed") {
			t.Fatalf("ListVMs() error = %q, want wrapped provider context", got)
		}
	})
}

func TestVMServiceGetVMManifestYAML_DelegatesToManifestProvider(t *testing.T) {
	t.Parallel()

	infra := &namespaceProvisioningProviderStub{manifestYAML: "apiVersion: kubevirt.io/v1\nkind: VirtualMachine\n"}
	svc := NewVMService(infra)

	got, err := svc.GetVMManifestYAML(t.Context(), "cluster-manifest", "team-manifest", "vm-manifest")
	if err != nil {
		t.Fatalf("GetVMManifestYAML() error = %v", err)
	}
	if got != infra.manifestYAML {
		t.Fatalf("GetVMManifestYAML() = %q, want fixture YAML", got)
	}
	if infra.manifestCalls != 1 || infra.lastCluster != "cluster-manifest" ||
		infra.lastNamespace != "team-manifest" || infra.lastName != "vm-manifest" {
		t.Fatalf(
			"manifest calls/args = calls:%d cluster:%q namespace:%q name:%q",
			infra.manifestCalls,
			infra.lastCluster,
			infra.lastNamespace,
			infra.lastName,
		)
	}
}

func TestVMServiceExecuteK8sUpdate_ValidatesRendersAndWrapsProviderErrors(t *testing.T) {
	t.Parallel()

	t.Run("renders name and delegates update", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{}
		svc := NewVMService(infra)
		spec := &domain.VMSpec{
			CPU:             2,
			CPURequest:      2,
			MemoryGi:        4,
			MemoryRequestGi: 4,
			DiskGB:          50,
			Image:           "docker://quay.io/containerdisks/ubuntu:22.04",
		}

		vm, err := svc.ExecuteK8sUpdate(t.Context(), "cluster-update", "team-update", "vm-update", spec)
		if err != nil {
			t.Fatalf("ExecuteK8sUpdate() error = %v", err)
		}
		if vm == nil || vm.Name != "vm-update" || vm.Namespace != "team-update" {
			t.Fatalf("ExecuteK8sUpdate() vm = %#v, want updated VM", vm)
		}
		if spec.Name != "vm-update" || strings.TrimSpace(spec.RenderedYAML) == "" {
			t.Fatalf("spec after update = name:%q rendered:%t, want name and rendered YAML", spec.Name, spec.RenderedYAML != "")
		}
		if infra.updateCalls != 1 || infra.lastCluster != "cluster-update" ||
			infra.lastNamespace != "team-update" || infra.lastName != "vm-update" {
			t.Fatalf(
				"UpdateVM calls/args = calls:%d cluster:%q namespace:%q name:%q",
				infra.updateCalls,
				infra.lastCluster,
				infra.lastNamespace,
				infra.lastName,
			)
		}
	})

	t.Run("validates required input", func(t *testing.T) {
		t.Parallel()
		svc := NewVMService(&namespaceProvisioningProviderStub{})
		if _, err := svc.ExecuteK8sUpdate(t.Context(), "cluster-update", "team-update", "vm-update", nil); err == nil {
			t.Fatal("ExecuteK8sUpdate() nil spec error = nil, want error")
		}
		if _, err := svc.ExecuteK8sUpdate(t.Context(), "cluster-update", "", "vm-update", &domain.VMSpec{}); err == nil {
			t.Fatal("ExecuteK8sUpdate() empty namespace error = nil, want error")
		}
		if _, err := svc.ExecuteK8sUpdate(t.Context(), "cluster-update", "team-update", "", &domain.VMSpec{}); err == nil {
			t.Fatal("ExecuteK8sUpdate() empty name error = nil, want error")
		}
	})

	t.Run("wraps update failure", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{updateErr: fmt.Errorf("ssa conflict")}
		svc := NewVMService(infra)

		_, err := svc.ExecuteK8sUpdate(t.Context(), "cluster-update", "team-update", "vm-update", &domain.VMSpec{
			Name:            "vm-update",
			CPU:             2,
			CPURequest:      2,
			MemoryGi:        4,
			MemoryRequestGi: 4,
			DiskGB:          50,
			Image:           "docker://quay.io/containerdisks/ubuntu:22.04",
		})
		if err == nil {
			t.Fatal("ExecuteK8sUpdate() expected provider error, got nil")
		}
		if got := err.Error(); !containsAll(got, "execute k8s update", "ssa conflict") {
			t.Fatalf("ExecuteK8sUpdate() error = %q, want wrapped provider context", got)
		}
	})
}

func TestVMServiceVMMutationMethods_ValidateAndDelegate(t *testing.T) {
	t.Parallel()

	mutation := &domain.VMMutation{
		Mode:      domain.VMMutationModePatch,
		PatchType: domain.VMMutationPatchTypeMerge,
		Payload:   []byte(`{"spec":{"running":true}}`),
	}

	t.Run("dry-run delegates", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{}
		svc := NewVMService(infra)

		err := svc.DryRunVMMutation(t.Context(), "cluster-mutate", "team-mutate", "vm-mutate", mutation)
		if err != nil {
			t.Fatalf("DryRunVMMutation() error = %v", err)
		}
		if infra.dryRunCalls != 1 || infra.lastMutation != mutation ||
			infra.lastCluster != "cluster-mutate" || infra.lastNamespace != "team-mutate" ||
			infra.lastName != "vm-mutate" {
			t.Fatalf(
				"DryRunVMMutation calls/args = calls:%d mutation:%p cluster:%q namespace:%q name:%q",
				infra.dryRunCalls,
				infra.lastMutation,
				infra.lastCluster,
				infra.lastNamespace,
				infra.lastName,
			)
		}
	})

	t.Run("dry-run validates input", func(t *testing.T) {
		t.Parallel()
		svc := NewVMService(&namespaceProvisioningProviderStub{})
		if err := svc.DryRunVMMutation(t.Context(), "cluster-mutate", "team-mutate", "vm-mutate", nil); err == nil {
			t.Fatal("DryRunVMMutation() nil mutation error = nil, want error")
		}
		if err := svc.DryRunVMMutation(t.Context(), "cluster-mutate", "", "vm-mutate", mutation); err == nil {
			t.Fatal("DryRunVMMutation() empty namespace error = nil, want error")
		}
		if err := svc.DryRunVMMutation(t.Context(), "cluster-mutate", "team-mutate", "", mutation); err == nil {
			t.Fatal("DryRunVMMutation() empty name error = nil, want error")
		}
	})

	t.Run("dry-run wraps provider failure", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{dryRunErr: fmt.Errorf("dry-run rejected")}
		svc := NewVMService(infra)

		err := svc.DryRunVMMutation(t.Context(), "cluster-mutate", "team-mutate", "vm-mutate", mutation)
		if err == nil {
			t.Fatal("DryRunVMMutation() expected provider error, got nil")
		}
		if got := err.Error(); !containsAll(got, "dry-run vm mutation", "dry-run rejected") {
			t.Fatalf("DryRunVMMutation() error = %q, want wrapped provider context", got)
		}
	})

	t.Run("execute delegates", func(t *testing.T) {
		t.Parallel()
		wantVM := &domain.VM{Name: "vm-mutate", Namespace: "team-mutate", Status: domain.VMStatusRunning}
		infra := &namespaceProvisioningProviderStub{mutationVM: wantVM}
		svc := NewVMService(infra)

		got, err := svc.ExecuteVMMutation(t.Context(), "cluster-mutate", "team-mutate", "vm-mutate", mutation)
		if err != nil {
			t.Fatalf("ExecuteVMMutation() error = %v", err)
		}
		if got != wantVM {
			t.Fatalf("ExecuteVMMutation() = %#v, want fixture VM", got)
		}
		if infra.mutationCalls != 1 || infra.lastMutation != mutation ||
			infra.lastCluster != "cluster-mutate" || infra.lastNamespace != "team-mutate" ||
			infra.lastName != "vm-mutate" {
			t.Fatalf(
				"ExecuteVMMutation calls/args = calls:%d mutation:%p cluster:%q namespace:%q name:%q",
				infra.mutationCalls,
				infra.lastMutation,
				infra.lastCluster,
				infra.lastNamespace,
				infra.lastName,
			)
		}
	})

	t.Run("execute validates input", func(t *testing.T) {
		t.Parallel()
		svc := NewVMService(&namespaceProvisioningProviderStub{})
		if _, err := svc.ExecuteVMMutation(t.Context(), "cluster-mutate", "team-mutate", "vm-mutate", nil); err == nil {
			t.Fatal("ExecuteVMMutation() nil mutation error = nil, want error")
		}
		if _, err := svc.ExecuteVMMutation(t.Context(), "cluster-mutate", "", "vm-mutate", mutation); err == nil {
			t.Fatal("ExecuteVMMutation() empty namespace error = nil, want error")
		}
		if _, err := svc.ExecuteVMMutation(t.Context(), "cluster-mutate", "team-mutate", "", mutation); err == nil {
			t.Fatal("ExecuteVMMutation() empty name error = nil, want error")
		}
	})

	t.Run("execute wraps provider failure", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{mutationErr: fmt.Errorf("patch failed")}
		svc := NewVMService(infra)

		_, err := svc.ExecuteVMMutation(t.Context(), "cluster-mutate", "team-mutate", "vm-mutate", mutation)
		if err == nil {
			t.Fatal("ExecuteVMMutation() expected provider error, got nil")
		}
		if got := err.Error(); !containsAll(got, "execute vm mutation", "patch failed") {
			t.Fatalf("ExecuteVMMutation() error = %q, want wrapped provider context", got)
		}
	})
}

func TestVMServicePowerAndDeleteMethods_DelegateToProvider(t *testing.T) {
	t.Parallel()

	t.Run("StartVM", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{}
		svc := NewVMService(infra)
		if err := svc.StartVM(t.Context(), "cluster-power", "team-power", "vm-power"); err != nil {
			t.Fatalf("StartVM() error = %v", err)
		}
		if infra.startCalls != 1 || infra.lastCluster != "cluster-power" ||
			infra.lastNamespace != "team-power" || infra.lastName != "vm-power" {
			t.Fatalf("StartVM calls/args = calls:%d cluster:%q namespace:%q name:%q",
				infra.startCalls, infra.lastCluster, infra.lastNamespace, infra.lastName)
		}
	})

	t.Run("StopVM", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{}
		svc := NewVMService(infra)
		if err := svc.StopVM(t.Context(), "cluster-power", "team-power", "vm-power"); err != nil {
			t.Fatalf("StopVM() error = %v", err)
		}
		if infra.stopCalls != 1 || infra.lastCluster != "cluster-power" ||
			infra.lastNamespace != "team-power" || infra.lastName != "vm-power" {
			t.Fatalf("StopVM calls/args = calls:%d cluster:%q namespace:%q name:%q",
				infra.stopCalls, infra.lastCluster, infra.lastNamespace, infra.lastName)
		}
	})

	t.Run("RestartVM", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{}
		svc := NewVMService(infra)
		if err := svc.RestartVM(t.Context(), "cluster-power", "team-power", "vm-power"); err != nil {
			t.Fatalf("RestartVM() error = %v", err)
		}
		if infra.restartCalls != 1 || infra.lastCluster != "cluster-power" ||
			infra.lastNamespace != "team-power" || infra.lastName != "vm-power" {
			t.Fatalf("RestartVM calls/args = calls:%d cluster:%q namespace:%q name:%q",
				infra.restartCalls, infra.lastCluster, infra.lastNamespace, infra.lastName)
		}
	})

	t.Run("DeleteVM", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{}
		svc := NewVMService(infra)
		if err := svc.DeleteVM(t.Context(), "cluster-power", "team-power", "vm-power"); err != nil {
			t.Fatalf("DeleteVM() error = %v", err)
		}
		if infra.deleteCalls != 1 || infra.lastCluster != "cluster-power" ||
			infra.lastNamespace != "team-power" || infra.lastName != "vm-power" {
			t.Fatalf("DeleteVM calls/args = calls:%d cluster:%q namespace:%q name:%q",
				infra.deleteCalls, infra.lastCluster, infra.lastNamespace, infra.lastName)
		}
	})

	t.Run("propagates provider error", func(t *testing.T) {
		t.Parallel()
		infra := &namespaceProvisioningProviderStub{startErr: fmt.Errorf("power denied")}
		svc := NewVMService(infra)
		if err := svc.StartVM(t.Context(), "cluster-power", "team-power", "vm-power"); err == nil ||
			!strings.Contains(err.Error(), "power denied") {
			t.Fatalf("StartVM() error = %v, want provider error", err)
		}
	})
}

func TestVMServiceMethods_ReturnConfiguredProviderError(t *testing.T) {
	t.Parallel()

	var nilService *VMService
	unconfiguredService := NewVMService(nil)
	mutation := vmMutationFixture()

	tests := []struct {
		name string
		svc  *VMService
		run  func(*testing.T, *VMService) error
	}{
		{
			name: "ValidateAndPrepare nil service",
			svc:  nilService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.ValidateAndPrepare(t.Context(), "cluster-unconfigured", "team-unconfigured", validVMServiceSpec("vm-unconfigured"))
				return err
			},
		},
		{
			name: "GetVM nil service",
			svc:  nilService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.GetVM(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured")
				return err
			},
		},
		{
			name: "GetVMManifestYAML nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.GetVMManifestYAML(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured")
				return err
			},
		},
		{
			name: "ListVMs nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.ListVMs(t.Context(), "cluster-unconfigured", "team-unconfigured", provider.ListOptions{})
				return err
			},
		},
		{
			name: "ExecuteK8sCreate nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.ExecuteK8sCreate(t.Context(), "cluster-unconfigured", "team-unconfigured", validVMServiceSpec("vm-unconfigured"))
				return err
			},
		},
		{
			name: "ExecuteK8sUpdate nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.ExecuteK8sUpdate(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured", validVMServiceSpec(""))
				return err
			},
		},
		{
			name: "DryRunVMMutation nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				return svc.DryRunVMMutation(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured", mutation)
			},
		},
		{
			name: "ExecuteVMMutation nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.ExecuteVMMutation(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured", mutation)
				return err
			},
		},
		{
			name: "GetStorageProfile nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.GetStorageProfile(t.Context(), "cluster-unconfigured", "gold-sc")
				return err
			},
		},
		{
			name: "StartVM nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				return svc.StartVM(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured")
			},
		},
		{
			name: "StopVM nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				return svc.StopVM(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured")
			},
		},
		{
			name: "RestartVM nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				return svc.RestartVM(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured")
			},
		},
		{
			name: "DeleteVM nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				return svc.DeleteVM(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured")
			},
		},
		{
			name: "OpenVNCStream nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.OpenVNCStream(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured")
				return err
			},
		},
		{
			name: "OpenSerialConsoleStream nil infra",
			svc:  unconfiguredService,
			run: func(t *testing.T, svc *VMService) error {
				_, err := svc.OpenSerialConsoleStream(t.Context(), "cluster-unconfigured", "team-unconfigured", "vm-unconfigured")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.run(t, tt.svc)
			if err == nil {
				t.Fatal("VMService method returned nil error, want infrastructure configuration error")
			}
			if got := err.Error(); !strings.Contains(got, "vm infrastructure provider is not configured") {
				t.Fatalf("VMService method error = %q, want infrastructure configuration error", got)
			}
		})
	}
}

func TestVMServiceValidateAndPrepare_PropagatesNamespaceProvisioningError(t *testing.T) {
	t.Parallel()

	infra := &namespaceProvisioningProviderStub{ensureErr: fmt.Errorf("forbidden")}
	svc := NewVMService(infra)

	_, err := svc.ValidateAndPrepare(t.Context(), "cluster-a", "team-a", &domain.VMSpec{
		Name:            "vm-c",
		CPU:             2,
		CPURequest:      2,
		MemoryGi:        4,
		MemoryRequestGi: 4,
		DiskGB:          50,
		Image:           "docker://quay.io/containerdisks/ubuntu:22.04",
	})
	if err == nil {
		t.Fatal("ValidateAndPrepare() expected namespace provisioning error, got nil")
	}
	if got := err.Error(); got == "" || !containsAll(got, "ensure namespace team-a", "forbidden") {
		t.Fatalf("ValidateAndPrepare() error = %q, want namespace provisioning context", got)
	}
}

func TestVMServiceOpenVNCStream_UsesProviderCapability(t *testing.T) {
	t.Parallel()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	infra := &namespaceProvisioningProviderStub{vncConn: serverConn}
	svc := NewVMService(infra)

	conn, err := svc.OpenVNCStream(t.Context(), "cluster-a", "team-a", "vm-a")
	if err != nil {
		t.Fatalf("OpenVNCStream() error = %v", err)
	}
	if conn == nil {
		t.Fatal("OpenVNCStream() returned nil conn")
	}
	if conn != serverConn {
		t.Fatal("OpenVNCStream() returned unexpected conn")
	}
}

func TestVMServiceOpenSerialConsoleStream_UsesProviderCapability(t *testing.T) {
	t.Parallel()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	infra := &namespaceProvisioningProviderStub{serialConn: serverConn}
	svc := NewVMService(infra)

	conn, err := svc.OpenSerialConsoleStream(t.Context(), "cluster-a", "team-a", "vm-a")
	if err != nil {
		t.Fatalf("OpenSerialConsoleStream() error = %v", err)
	}
	if conn == nil {
		t.Fatal("OpenSerialConsoleStream() returned nil conn")
	}
	if conn != serverConn {
		t.Fatal("OpenSerialConsoleStream() returned unexpected conn")
	}
}

func containsAll(input string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(input, part) {
			return false
		}
	}
	return true
}

func alreadyExistsVMError(name string) error {
	return fmt.Errorf(
		"apply vm: %w",
		apierrors.NewAlreadyExists(schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachines"}, name),
	)
}

func validVMServiceSpec(name string) *domain.VMSpec {
	return &domain.VMSpec{
		Name:            name,
		CPU:             2,
		CPURequest:      2,
		MemoryGi:        4,
		MemoryRequestGi: 4,
		DiskGB:          50,
		Image:           "docker://quay.io/containerdisks/ubuntu:22.04",
	}
}

func vmMutationFixture() *domain.VMMutation {
	return &domain.VMMutation{
		Mode:      domain.VMMutationModePatch,
		PatchType: domain.VMMutationPatchTypeMerge,
		Payload:   []byte(`{"spec":{"running":true}}`),
	}
}
