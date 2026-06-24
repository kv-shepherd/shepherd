package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	kubevirtv1 "kubevirt.io/api/core/v1"
	instancetypev1beta1 "kubevirt.io/api/instancetype/v1beta1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"kv-shepherd.io/shepherd/internal/domain"
)

// validVMYAML is a minimal valid VirtualMachine YAML for testing.
const validVMYAML = `
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: test-vm
  namespace: test-ns
  labels:
    env: test
spec:
  runStrategy: Always
  template:
    metadata:
      labels:
        env: test
    spec:
      domain:
        cpu:
          cores: 4
        resources:
          requests:
            cpu: "4"
            memory: "8Gi"
          limits:
            cpu: "4"
            memory: "8Gi"
        devices:
          disks:
            - name: rootfs
              disk:
                bus: virtio
      volumes:
        - name: rootfs
          containerDisk:
            image: docker.io/kubevirt/centos:7
`

func vmYAMLWithEventID(eventID string) string {
	return strings.ReplaceAll(
		validVMYAML,
		"    env: test\n",
		fmt.Sprintf("    env: test\n    %s: %s\n", domain.ShepherdEventIDLabel, eventID),
	)
}

func TestKubevirtSSAApplier_ApplyYAML(t *testing.T) {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme)

	// Intercept the Patch call to verify SSA is used.
	var capturedPatch []byte
	var capturedPatchType types.PatchType
	var capturedName string

	dynClient.PrependReactor("patch", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patchAction, ok := action.(k8stesting.PatchActionImpl)
		if !ok {
			return false, nil, nil
		}
		capturedPatch = patchAction.GetPatch()
		capturedPatchType = patchAction.GetPatchType()
		capturedName = patchAction.GetName()

		// Return the object from the patch body as the result.
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(capturedPatch, &obj.Object); err != nil {
			return true, nil, fmt.Errorf("unmarshal patch: %w", err)
		}
		return true, obj, nil
	})

	applier := NewKubevirtSSAApplier(dynClient)

	result, err := applier.ApplyYAML(context.Background(), "test-ns", []byte(validVMYAML))
	if err != nil {
		t.Fatalf("ApplyYAML returned error: %v", err)
	}

	// Verify SSA patch type is used (ApplyPatchType).
	if capturedPatchType != types.ApplyPatchType {
		t.Fatalf("expected ApplyPatchType, got %q", capturedPatchType)
	}

	// Verify the patch targets the correct resource name.
	if capturedName != "test-vm" {
		t.Fatalf("expected patch target name=test-vm, got %q", capturedName)
	}

	// Verify the returned object.
	if result.GetName() != "test-vm" {
		t.Fatalf("expected name=test-vm, got %q", result.GetName())
	}
	if result.GetNamespace() != "test-ns" {
		t.Fatalf("expected namespace=test-ns, got %q", result.GetNamespace())
	}

	// Verify the patch body contains valid JSON with expected fields.
	var patchMap map[string]interface{}
	if err := json.Unmarshal(capturedPatch, &patchMap); err != nil {
		t.Fatalf("patch body is not valid JSON: %v", err)
	}
	if patchMap["apiVersion"] != "kubevirt.io/v1" {
		t.Fatalf("expected apiVersion=kubevirt.io/v1 in patch, got %v", patchMap["apiVersion"])
	}
	if patchMap["kind"] != "VirtualMachine" {
		t.Fatalf("expected kind=VirtualMachine in patch, got %v", patchMap["kind"])
	}
}

func TestKubevirtSSAApplier_ApplyYAML_MissingName(t *testing.T) {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
	applier := NewKubevirtSSAApplier(dynClient)

	noNameYAML := `
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  namespace: test-ns
spec:
  runStrategy: Always
`

	_, err := applier.ApplyYAML(context.Background(), "test-ns", []byte(noNameYAML))
	if err == nil {
		t.Fatalf("expected error for missing name, got nil")
	}
}

func TestKubevirtSSAApplier_ApplyYAML_InvalidYAML(t *testing.T) {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
	applier := NewKubevirtSSAApplier(dynClient)

	_, err := applier.ApplyYAML(context.Background(), "test-ns", []byte("not: valid: yaml: {{{}}}"))
	if err == nil {
		t.Fatalf("expected error for invalid yaml, got nil")
	}
}

func TestKubevirtSSAApplier_DryRunApplyYAML(t *testing.T) {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme)

	var patchCalled bool

	dynClient.PrependReactor("patch", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patchCalled = true
		patchAction, ok := action.(k8stesting.PatchActionImpl)
		if !ok {
			return false, nil, nil
		}

		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(patchAction.GetPatch(), &obj.Object); err != nil {
			return true, nil, fmt.Errorf("unmarshal patch: %w", err)
		}
		return true, obj, nil
	})

	applier := NewKubevirtSSAApplier(dynClient)

	err := applier.DryRunApplyYAML(context.Background(), "test-ns", []byte(validVMYAML))
	if err != nil {
		t.Fatalf("DryRunApplyYAML returned error: %v", err)
	}

	// Verify the patch was called (regardless of DryRun propagation in fake client).
	if !patchCalled {
		t.Fatalf("expected patch to be called for DryRun")
	}
}

func TestKubevirtSSAApplier_ApplyClusterScopedYAML(t *testing.T) {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme)

	var capturedPatch []byte
	var capturedPatchType types.PatchType
	var capturedName string

	dynClient.PrependReactor("patch", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patchAction, ok := action.(k8stesting.PatchActionImpl)
		if !ok {
			return false, nil, nil
		}
		capturedPatch = patchAction.GetPatch()
		capturedPatchType = patchAction.GetPatchType()
		capturedName = patchAction.GetName()

		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(capturedPatch, &obj.Object); err != nil {
			return true, nil, fmt.Errorf("unmarshal patch: %w", err)
		}
		return true, obj, nil
	})

	applier := NewKubevirtSSAApplier(dynClient)

	result, err := applier.ApplyClusterScopedYAML(context.Background(), namespaceGVR, []byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: team-a
`))
	if err != nil {
		t.Fatalf("ApplyClusterScopedYAML returned error: %v", err)
	}
	if capturedPatchType != types.ApplyPatchType {
		t.Fatalf("expected ApplyPatchType, got %q", capturedPatchType)
	}
	if capturedName != "team-a" {
		t.Fatalf("expected patch target name=team-a, got %q", capturedName)
	}
	if result.GetName() != "team-a" {
		t.Fatalf("expected name=team-a, got %q", result.GetName())
	}
}

func TestKubevirtSSAApplier_GVR(t *testing.T) {
	// Verify the GVR constants match expected KubeVirt API group.
	expected := schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}
	if vmGVR != expected {
		t.Fatalf("vmGVR mismatch: got %v, expected %v", vmGVR, expected)
	}
}

func TestKubevirtSSAApplier_FieldOwnerConstant(t *testing.T) {
	// Verify the FieldOwner constant is set correctly.
	if FieldOwner != "kubevirt-shepherd" {
		t.Fatalf("expected FieldOwner=kubevirt-shepherd, got %q", FieldOwner)
	}
}

func TestKubeVirtProviderCreateVM_AppliesWhenNoExistingVM(t *testing.T) {
	eventID := "event-create-new"
	vmClient := &createGuardVMClient{
		gets: []createGuardGetResult{
			{err: apierrors.NewNotFound(schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachines"}, "test-vm")},
			{vm: kubevirtVMWithEventID("test-vm", "test-ns", eventID)},
		},
	}
	ssaClient := &createGuardSSAClient{}
	provider := NewKubeVirtProvider(func(string) (KubeVirtClusterClient, error) {
		return &timeoutProbeClusterClient{vm: vmClient, ssa: ssaClient}, nil
	}, time.Minute)

	got, err := provider.CreateVM(context.Background(), "cluster-a", "test-ns", &domain.VMSpec{
		Name:         "test-vm",
		RenderedYAML: vmYAMLWithEventID(eventID),
		Labels:       map[string]string{domain.ShepherdEventIDLabel: eventID},
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	if got == nil || got.Name != "test-vm" {
		t.Fatalf("CreateVM() = %#v, want test-vm", got)
	}
	if !ssaClient.applyCalled {
		t.Fatal("CreateVM() did not call SSA apply for absent VM")
	}
	if vmClient.getCalls != 2 {
		t.Fatalf("VM Get calls = %d, want preflight + readback", vmClient.getCalls)
	}
}

func TestKubeVirtProviderCreateVM_ReturnsExistingOwnedVMWithoutApplying(t *testing.T) {
	eventID := "event-owned"
	vmClient := &createGuardVMClient{
		gets: []createGuardGetResult{
			{vm: kubevirtVMWithEventID("test-vm", "test-ns", eventID)},
		},
	}
	ssaClient := &createGuardSSAClient{}
	provider := NewKubeVirtProvider(func(string) (KubeVirtClusterClient, error) {
		return &timeoutProbeClusterClient{vm: vmClient, ssa: ssaClient}, nil
	}, time.Minute)

	got, err := provider.CreateVM(context.Background(), "cluster-a", "test-ns", &domain.VMSpec{
		Name:         "test-vm",
		RenderedYAML: vmYAMLWithEventID(eventID),
		Labels:       map[string]string{domain.ShepherdEventIDLabel: eventID},
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	if got == nil || got.Name != "test-vm" || got.Spec.Labels[domain.ShepherdEventIDLabel] != eventID {
		t.Fatalf("CreateVM() = %#v, want existing VM with matching event label", got)
	}
	if ssaClient.applyCalled {
		t.Fatal("CreateVM() applied YAML even though an owned VM already existed")
	}
	if vmClient.getCalls != 1 {
		t.Fatalf("VM Get calls = %d, want ownership preflight only", vmClient.getCalls)
	}
}

func TestKubeVirtProviderCreateVM_RejectsExistingVMWithDifferentEventLabel(t *testing.T) {
	eventID := "event-requested"
	vmClient := &createGuardVMClient{
		gets: []createGuardGetResult{
			{vm: kubevirtVMWithEventID("test-vm", "test-ns", "event-other")},
		},
	}
	ssaClient := &createGuardSSAClient{}
	provider := NewKubeVirtProvider(func(string) (KubeVirtClusterClient, error) {
		return &timeoutProbeClusterClient{vm: vmClient, ssa: ssaClient}, nil
	}, time.Minute)

	_, err := provider.CreateVM(context.Background(), "cluster-a", "test-ns", &domain.VMSpec{
		Name:         "test-vm",
		RenderedYAML: vmYAMLWithEventID(eventID),
		Labels:       map[string]string{domain.ShepherdEventIDLabel: eventID},
	})
	if err == nil {
		t.Fatal("CreateVM() expected ownership error, got nil")
	}
	if !apierrors.IsAlreadyExists(err) {
		t.Fatalf("CreateVM() error = %v, want wrapped AlreadyExists", err)
	}
	if !strings.Contains(err.Error(), "does not match requested") {
		t.Fatalf("CreateVM() error = %q, want ownership mismatch context", err)
	}
	if ssaClient.applyCalled {
		t.Fatal("CreateVM() applied YAML after detecting an unowned existing VM")
	}
}

func TestKubevirtSSAApplier_DecodeAndMarshal_JSONRoundtrip(t *testing.T) {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
	applier := NewKubevirtSSAApplier(dynClient)

	obj, jsonData, err := applier.decodeAndMarshal([]byte(validVMYAML))
	if err != nil {
		t.Fatalf("decodeAndMarshal returned error: %v", err)
	}

	// Verify the decoded object has the expected fields.
	if obj.GetName() != "test-vm" {
		t.Fatalf("expected name=test-vm, got %q", obj.GetName())
	}
	if obj.GetNamespace() != "test-ns" {
		t.Fatalf("expected namespace=test-ns, got %q", obj.GetNamespace())
	}
	if obj.GetKind() != "VirtualMachine" {
		t.Fatalf("expected kind=VirtualMachine, got %q", obj.GetKind())
	}

	// Verify JSON roundtrip: the JSON should be valid and contain the expected fields.
	var decoded map[string]interface{}
	if err := json.NewDecoder(bytes.NewReader(jsonData)).Decode(&decoded); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if decoded["apiVersion"] != "kubevirt.io/v1" {
		t.Fatalf("expected apiVersion=kubevirt.io/v1, got %v", decoded["apiVersion"])
	}

	// Verify nested fields survive JSON roundtrip.
	spec, ok := decoded["spec"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected spec to be a map")
	}
	if spec["runStrategy"] != "Always" {
		t.Fatalf("expected runStrategy=Always, got %v", spec["runStrategy"])
	}
}

func TestResolveNodePrimaryIP_PrefersInternalIP(t *testing.T) {
	t.Parallel()

	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeExternalIP, Address: "203.0.113.10"},
				{Type: corev1.NodeInternalIP, Address: "10.1.2.3"},
			},
		},
	}

	if got := resolveNodePrimaryIP(node); got != "10.1.2.3" {
		t.Fatalf("resolveNodePrimaryIP() = %q, want %q", got, "10.1.2.3")
	}
}

func TestEnrichVMUpdateManifestWithCurrentDevices(t *testing.T) {
	patchYAML := []byte(`
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: test-vm
  namespace: test-ns
spec:
  template:
    spec:
      domain:
        resources:
          limits:
            memory: 4Gi
`)

	currentVM := &kubevirtv1.VirtualMachine{}
	if err := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(validVMYAML)), 4096).Decode(currentVM); err != nil {
		t.Fatalf("decode current vm yaml: %v", err)
	}

	client := &fakeClusterClientForUpdateManifest{
		vm: currentVM,
	}

	got, err := enrichVMUpdateManifestWithCurrentDevices(
		context.Background(),
		client,
		"test-ns",
		"test-vm",
		patchYAML,
	)
	if err != nil {
		t.Fatalf("enrichVMUpdateManifestWithCurrentDevices returned error: %v", err)
	}

	obj := &unstructured.Unstructured{}
	if unmarshalErr := json.Unmarshal(got, &obj.Object); unmarshalErr != nil {
		t.Fatalf("decode enriched manifest json: %v", unmarshalErr)
	}

	limits, found, err := unstructured.NestedStringMap(
		obj.Object,
		"spec", "template", "spec", "domain", "resources", "limits",
	)
	if err != nil || !found {
		t.Fatalf("expected limits in enriched manifest, found=%v err=%v", found, err)
	}
	if diff := cmp.Diff(map[string]string{"memory": "4Gi"}, limits); diff != "" {
		t.Fatalf("limits mismatch (-want +got):\n%s", diff)
	}

	disks, found, err := unstructured.NestedSlice(
		obj.Object,
		"spec", "template", "spec", "domain", "devices", "disks",
	)
	if err != nil || !found {
		t.Fatalf("expected devices.disks in enriched manifest, found=%v err=%v", found, err)
	}
	if len(disks) != 1 {
		t.Fatalf("expected 1 disk in enriched manifest, got %d", len(disks))
	}
}

func TestResolveVMMutationPatchType(t *testing.T) {
	t.Parallel()

	got, err := resolveVMMutationPatchType(domain.VMMutationPatchTypeMerge)
	if err != nil {
		t.Fatalf("resolveVMMutationPatchType(merge) error = %v", err)
	}
	if got != types.MergePatchType {
		t.Fatalf("resolveVMMutationPatchType(merge) = %q, want %q", got, types.MergePatchType)
	}

	got, err = resolveVMMutationPatchType(domain.VMMutationPatchTypeJSON)
	if err != nil {
		t.Fatalf("resolveVMMutationPatchType(json) error = %v", err)
	}
	if got != types.JSONPatchType {
		t.Fatalf("resolveVMMutationPatchType(json) = %q, want %q", got, types.JSONPatchType)
	}
}

func TestKubeVirtProvider_DryRunVMMutation_UsesTypedPatchWithDryRun(t *testing.T) {
	t.Parallel()

	vmClient := &capturingPatchVMClient{
		result: &kubevirtv1.VirtualMachine{
			ObjectMeta: k8smetav1.ObjectMeta{
				Name:      "vm-a",
				Namespace: "team-a",
			},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		if clusterName != "cluster-a" {
			t.Fatalf("clientFactory cluster = %q, want %q", clusterName, "cluster-a")
		}
		return &capturingPatchClusterClient{vm: vmClient}, nil
	}, 0)

	mutation := &domain.VMMutation{
		Mode:      domain.VMMutationModePatch,
		PatchType: domain.VMMutationPatchTypeMerge,
		Payload:   []byte(`{"spec":{"template":{"spec":{"domain":{"memory":{"guest":"8Gi"}}}}}}`),
	}
	if err := provider.DryRunVMMutation(context.Background(), "cluster-a", "team-a", "vm-a", mutation); err != nil {
		t.Fatalf("DryRunVMMutation() error = %v", err)
	}
	if !vmClient.patchCalled {
		t.Fatal("Patch() was not called during DryRunVMMutation")
	}
	if vmClient.patchType != types.MergePatchType {
		t.Fatalf("Patch() type = %q, want %q", vmClient.patchType, types.MergePatchType)
	}
	if diff := cmp.Diff([]string{k8smetav1.DryRunAll}, vmClient.patchOpts.DryRun); diff != "" {
		t.Fatalf("Patch() dryRun mismatch (-want +got):\n%s", diff)
	}
	if vmClient.namespace != "team-a" || vmClient.name != "vm-a" {
		t.Fatalf("Patch() target = %s/%s, want team-a/vm-a", vmClient.namespace, vmClient.name)
	}
	if !bytes.Equal(vmClient.patchPayload, mutation.Payload) {
		t.Fatalf("Patch() payload = %q, want %q", vmClient.patchPayload, mutation.Payload)
	}
}

func TestKubeVirtProvider_ExecuteVMMutation_UsesTypedPatchWithoutDryRun(t *testing.T) {
	t.Parallel()

	vmClient := &capturingPatchVMClient{
		result: &kubevirtv1.VirtualMachine{
			ObjectMeta: k8smetav1.ObjectMeta{
				Name:      "vm-a",
				Namespace: "team-a",
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: ptrRunStrategy(kubevirtv1.RunStrategyAlways),
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							CPU: &kubevirtv1.CPU{
								Sockets: 2,
								Cores:   1,
								Threads: 1,
							},
						},
					},
				},
			},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		return &capturingPatchClusterClient{vm: vmClient}, nil
	}, 0)

	mutation := &domain.VMMutation{
		Mode:      domain.VMMutationModePatch,
		PatchType: domain.VMMutationPatchTypeMerge,
		Payload:   []byte(`{"spec":{"runStrategy":"Always"}}`),
	}
	vm, err := provider.ExecuteVMMutation(context.Background(), "cluster-a", "team-a", "vm-a", mutation)
	if err != nil {
		t.Fatalf("ExecuteVMMutation() error = %v", err)
	}
	if !vmClient.patchCalled {
		t.Fatal("Patch() was not called during ExecuteVMMutation")
	}
	if len(vmClient.patchOpts.DryRun) != 0 {
		t.Fatalf("Patch() dryRun = %v, want empty", vmClient.patchOpts.DryRun)
	}
	if vm == nil || vm.Name != "vm-a" || vm.Namespace != "team-a" {
		t.Fatalf("ExecuteVMMutation() vm = %#v, want mapped vm-a/team-a", vm)
	}
}

func TestKubeVirtProvider_StartVM_PatchesHaltedRunStrategyToAlways(t *testing.T) {
	t.Parallel()

	vmClient := &capturingPatchVMClient{
		result: &kubevirtv1.VirtualMachine{
			ObjectMeta: k8smetav1.ObjectMeta{Name: "vm-a", Namespace: "team-a"},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
			},
			Status: kubevirtv1.VirtualMachineStatus{
				PrintableStatus: kubevirtv1.VirtualMachineStatusStopped,
			},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		return &capturingPatchClusterClient{vm: vmClient}, nil
	}, 0)

	if err := provider.StartVM(context.Background(), "cluster-a", "team-a", "vm-a"); err != nil {
		t.Fatalf("StartVM() error = %v", err)
	}
	if vmClient.startCalls != 0 {
		t.Fatalf("Start() calls = %d, want 0 for RunStrategy controlled VM", vmClient.startCalls)
	}
	assertRunStrategyPatch(t, vmClient, kubevirtv1.RunStrategyAlways)
}

func TestKubeVirtProvider_StartVM_CyclesStoppedAlwaysRunStrategy(t *testing.T) {
	t.Parallel()

	vmClient := &capturingPatchVMClient{
		result: &kubevirtv1.VirtualMachine{
			ObjectMeta: k8smetav1.ObjectMeta{Name: "vm-a", Namespace: "team-a"},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: ptrRunStrategy(kubevirtv1.RunStrategyAlways),
			},
			Status: kubevirtv1.VirtualMachineStatus{
				PrintableStatus: kubevirtv1.VirtualMachineStatusStopped,
			},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		return &capturingPatchClusterClient{vm: vmClient}, nil
	}, 0)

	if err := provider.StartVM(context.Background(), "cluster-a", "team-a", "vm-a"); err != nil {
		t.Fatalf("StartVM() error = %v", err)
	}
	if vmClient.startCalls != 0 {
		t.Fatalf("Start() calls = %d, want 0 for RunStrategy controlled VM", vmClient.startCalls)
	}
	if len(vmClient.patchPayloads) != 2 {
		t.Fatalf("Patch() calls = %d, want 2", len(vmClient.patchPayloads))
	}
	assertRunStrategyPatchPayload(t, vmClient.patchPayloads[0], kubevirtv1.RunStrategyHalted)
	assertRunStrategyPatchPayload(t, vmClient.patchPayloads[1], kubevirtv1.RunStrategyAlways)
}

func TestKubeVirtProvider_StartVM_FallsBackToRunStrategyWhenManualStartUnsupported(t *testing.T) {
	t.Parallel()

	vmClient := &capturingPatchVMClient{
		startErr: fmt.Errorf("Always does not support manual start requests"),
		result: &kubevirtv1.VirtualMachine{
			ObjectMeta: k8smetav1.ObjectMeta{Name: "vm-a", Namespace: "team-a"},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: ptrRunStrategy(kubevirtv1.RunStrategyManual),
			},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		return &capturingPatchClusterClient{vm: vmClient}, nil
	}, 0)

	if err := provider.StartVM(context.Background(), "cluster-a", "team-a", "vm-a"); err != nil {
		t.Fatalf("StartVM() error = %v", err)
	}
	if vmClient.startCalls != 1 {
		t.Fatalf("Start() calls = %d, want 1 before fallback", vmClient.startCalls)
	}
	assertRunStrategyPatch(t, vmClient, kubevirtv1.RunStrategyAlways)
}

func TestKubeVirtProvider_StopVM_PatchesAlwaysRunStrategyToHalted(t *testing.T) {
	t.Parallel()

	vmClient := &capturingPatchVMClient{
		result: &kubevirtv1.VirtualMachine{
			ObjectMeta: k8smetav1.ObjectMeta{Name: "vm-a", Namespace: "team-a"},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: ptrRunStrategy(kubevirtv1.RunStrategyAlways),
			},
			Status: kubevirtv1.VirtualMachineStatus{
				PrintableStatus: kubevirtv1.VirtualMachineStatusRunning,
			},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		return &capturingPatchClusterClient{vm: vmClient}, nil
	}, 0)

	if err := provider.StopVM(context.Background(), "cluster-a", "team-a", "vm-a"); err != nil {
		t.Fatalf("StopVM() error = %v", err)
	}
	if vmClient.stopCalls != 0 {
		t.Fatalf("Stop() calls = %d, want 0 for RunStrategy controlled VM", vmClient.stopCalls)
	}
	assertRunStrategyPatch(t, vmClient, kubevirtv1.RunStrategyHalted)
}

func TestKubeVirtProvider_RestartVM_FallsBackToRunStrategyCycleWhenManualRestartUnsupported(t *testing.T) {
	t.Parallel()

	vmClient := &capturingPatchVMClient{
		restartErr: fmt.Errorf("Always does not support manual restart requests"),
		result: &kubevirtv1.VirtualMachine{
			ObjectMeta: k8smetav1.ObjectMeta{Name: "vm-a", Namespace: "team-a"},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: ptrRunStrategy(kubevirtv1.RunStrategyAlways),
			},
			Status: kubevirtv1.VirtualMachineStatus{
				PrintableStatus: kubevirtv1.VirtualMachineStatusRunning,
			},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		return &capturingPatchClusterClient{vm: vmClient}, nil
	}, 0)

	if err := provider.RestartVM(context.Background(), "cluster-a", "team-a", "vm-a"); err != nil {
		t.Fatalf("RestartVM() error = %v", err)
	}
	if vmClient.restartCalls != 1 {
		t.Fatalf("Restart() calls = %d, want 1 before fallback", vmClient.restartCalls)
	}
	if len(vmClient.patchPayloads) != 2 {
		t.Fatalf("Patch() calls = %d, want 2", len(vmClient.patchPayloads))
	}
	assertRunStrategyPatchPayload(t, vmClient.patchPayloads[0], kubevirtv1.RunStrategyHalted)
	assertRunStrategyPatchPayload(t, vmClient.patchPayloads[1], kubevirtv1.RunStrategyAlways)
}

func TestKubeVirtProvider_GetVMUsesOperationTimeout(t *testing.T) {
	t.Parallel()

	vmClient := &timeoutProbeVMClient{
		vm: &kubevirtv1.VirtualMachine{
			ObjectMeta: k8smetav1.ObjectMeta{Name: "vm-a", Namespace: "team-a"},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
			},
		},
	}
	vmiClient := &timeoutProbeVMIClient{
		vmi: &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: k8smetav1.ObjectMeta{Name: "vm-a", Namespace: "team-a"},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		return &timeoutProbeClusterClient{
			vm:  vmClient,
			vmi: vmiClient,
		}, nil
	}, 2*time.Second)

	if _, err := provider.GetVM(context.Background(), "cluster-a", "team-a", "vm-a"); err != nil {
		t.Fatalf("GetVM() error = %v", err)
	}
	assertContextDeadlineWithin(vmClient.getCtx, t)
	assertContextDeadlineWithin(vmiClient.getCtx, t)
}

func TestKubeVirtProvider_ListVMsUsesOperationTimeoutForEnrichment(t *testing.T) {
	t.Parallel()

	vmClient := &timeoutProbeVMClient{
		list: &kubevirtv1.VirtualMachineList{
			Items: []kubevirtv1.VirtualMachine{{
				ObjectMeta: k8smetav1.ObjectMeta{Name: "vm-a", Namespace: "team-a"},
				Status:     kubevirtv1.VirtualMachineStatus{PrintableStatus: kubevirtv1.VirtualMachineStatusRunning},
			}},
		},
	}
	vmiClient := &timeoutProbeVMIClient{
		list: &kubevirtv1.VirtualMachineInstanceList{
			Items: []kubevirtv1.VirtualMachineInstance{{
				ObjectMeta: k8smetav1.ObjectMeta{Name: "vm-a", Namespace: "team-a"},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					NodeName: "node-a",
				},
			}},
		},
	}
	nodeClient := &timeoutProbeNodeClient{
		node: &corev1.Node{
			ObjectMeta: k8smetav1.ObjectMeta{Name: "node-a"},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.10"}},
			},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		return &timeoutProbeClusterClient{
			vm:    vmClient,
			vmi:   vmiClient,
			nodes: nodeClient,
		}, nil
	}, 2*time.Second)

	list, err := provider.ListVMs(context.Background(), "cluster-a", "team-a", ListOptions{})
	if err != nil {
		t.Fatalf("ListVMs() error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].HostIP != "10.0.0.10" {
		t.Fatalf("ListVMs() host enrichment = %#v, want host IP 10.0.0.10", list.Items)
	}
	assertContextDeadlineWithin(vmClient.listCtx, t)
	assertContextDeadlineWithin(vmiClient.listCtx, t)
	assertContextDeadlineWithin(nodeClient.getCtx, t)
}

func TestKubeVirtProvider_ValidateSpecUsesOperationTimeoutForDryRun(t *testing.T) {
	t.Parallel()

	ssaClient := &timeoutProbeSSAClient{}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		return &timeoutProbeClusterClient{ssa: ssaClient}, nil
	}, 2*time.Second)

	result, err := provider.ValidateSpec(context.Background(), "cluster-a", "team-a", &domain.VMSpec{
		RenderedYAML: validVMYAML,
	})
	if err != nil {
		t.Fatalf("ValidateSpec() error = %v", err)
	}
	if result == nil || !result.Valid {
		t.Fatalf("ValidateSpec() result = %#v, want valid", result)
	}
	assertContextDeadlineWithin(ssaClient.dryRunCtx, t)
}

func TestKubeVirtProvider_ListInstanceTypeCatalog(t *testing.T) {
	t.Parallel()

	memory := resource.MustParse("8Gi")
	clusterMemory := resource.MustParse("16Gi")
	catalog := &catalogProbeClusterClient{
		instanceTypes: &instancetypev1beta1.VirtualMachineInstancetypeList{
			Items: []instancetypev1beta1.VirtualMachineInstancetype{{
				ObjectMeta: k8smetav1.ObjectMeta{Name: "small", Namespace: "team-a"},
				Spec: instancetypev1beta1.VirtualMachineInstancetypeSpec{
					CPU:    instancetypev1beta1.CPUInstancetype{Guest: 2},
					Memory: instancetypev1beta1.MemoryInstancetype{Guest: memory},
				},
			}},
		},
		clusterTypes: &instancetypev1beta1.VirtualMachineClusterInstancetypeList{
			Items: []instancetypev1beta1.VirtualMachineClusterInstancetype{{
				ObjectMeta: k8smetav1.ObjectMeta{Name: "large"},
				Spec: instancetypev1beta1.VirtualMachineInstancetypeSpec{
					CPU:    instancetypev1beta1.CPUInstancetype{Guest: 8},
					Memory: instancetypev1beta1.MemoryInstancetype{Guest: clusterMemory},
				},
			}},
		},
		preferences: &instancetypev1beta1.VirtualMachinePreferenceList{
			Items: []instancetypev1beta1.VirtualMachinePreference{{
				ObjectMeta: k8smetav1.ObjectMeta{Name: "linux", Namespace: "team-a"},
			}},
		},
		clusterPreferences: &instancetypev1beta1.VirtualMachineClusterPreferenceList{
			Items: []instancetypev1beta1.VirtualMachineClusterPreference{{
				ObjectMeta: k8smetav1.ObjectMeta{Name: "rhel"},
			}},
		},
	}
	provider := NewKubeVirtProvider(func(clusterName string) (KubeVirtClusterClient, error) {
		if clusterName != "cluster-a" {
			t.Fatalf("clientFactory cluster = %q, want cluster-a", clusterName)
		}
		return catalog, nil
	}, 2*time.Second)

	instanceTypes, err := provider.ListInstanceTypes(context.Background(), "cluster-a", "team-a")
	if err != nil {
		t.Fatalf("ListInstanceTypes() error = %v", err)
	}
	if diff := cmp.Diff([]*domain.InstanceType{{Name: "small", CPU: 2, Memory: "8Gi"}}, instanceTypes); diff != "" {
		t.Fatalf("ListInstanceTypes() mismatch (-want +got):\n%s", diff)
	}
	if catalog.instanceTypesNamespace != "team-a" {
		t.Fatalf("ListInstanceTypes() namespace = %q, want team-a", catalog.instanceTypesNamespace)
	}

	clusterTypes, err := provider.ListClusterInstanceTypes(context.Background(), "cluster-a")
	if err != nil {
		t.Fatalf("ListClusterInstanceTypes() error = %v", err)
	}
	if diff := cmp.Diff([]*domain.InstanceType{{Name: "large", CPU: 8, Memory: "16Gi"}}, clusterTypes); diff != "" {
		t.Fatalf("ListClusterInstanceTypes() mismatch (-want +got):\n%s", diff)
	}

	preferences, err := provider.ListPreferences(context.Background(), "cluster-a", "team-a")
	if err != nil {
		t.Fatalf("ListPreferences() error = %v", err)
	}
	if diff := cmp.Diff([]*domain.Preference{{Name: "linux"}}, preferences); diff != "" {
		t.Fatalf("ListPreferences() mismatch (-want +got):\n%s", diff)
	}
	if catalog.preferencesNamespace != "team-a" {
		t.Fatalf("ListPreferences() namespace = %q, want team-a", catalog.preferencesNamespace)
	}

	clusterPreferences, err := provider.ListClusterPreferences(context.Background(), "cluster-a")
	if err != nil {
		t.Fatalf("ListClusterPreferences() error = %v", err)
	}
	if diff := cmp.Diff([]*domain.Preference{{Name: "rhel"}}, clusterPreferences); diff != "" {
		t.Fatalf("ListClusterPreferences() mismatch (-want +got):\n%s", diff)
	}

	assertContextDeadlineWithin(catalog.instanceTypesCtx, t)
	assertContextDeadlineWithin(catalog.clusterTypesCtx, t)
	assertContextDeadlineWithin(catalog.preferencesCtx, t)
	assertContextDeadlineWithin(catalog.clusterPreferencesCtx, t)
}

func assertContextDeadlineWithin(ctx context.Context, t *testing.T) {
	t.Helper()
	if ctx == nil {
		t.Fatal("captured context is nil")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("captured context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.Fatalf("captured context deadline already expired: remaining=%s", remaining)
	}
	if remaining > 2*time.Second {
		t.Fatalf("captured context deadline remaining=%s, want <= %s", remaining, 2*time.Second)
	}
}

type fakeClusterClientForUpdateManifest struct {
	vm *kubevirtv1.VirtualMachine
}

func (f *fakeClusterClientForUpdateManifest) VM() VirtualMachineClient {
	return &fakeVMClientForUpdateManifest{vm: f.vm}
}

func (f *fakeClusterClientForUpdateManifest) VMI() VirtualMachineInstanceClient { return nil }
func (f *fakeClusterClientForUpdateManifest) DataVolume() DataVolumeClient      { return nil }
func (f *fakeClusterClientForUpdateManifest) StorageProfile() StorageProfileClient {
	return nil
}
func (f *fakeClusterClientForUpdateManifest) PVC() PersistentVolumeClaimClient { return nil }
func (f *fakeClusterClientForUpdateManifest) StorageClass() StorageClassClient { return nil }
func (f *fakeClusterClientForUpdateManifest) Events() EventClient              { return nil }
func (f *fakeClusterClientForUpdateManifest) Namespaces() NamespaceClient      { return nil }
func (f *fakeClusterClientForUpdateManifest) Nodes() NodeClient                { return nil }
func (f *fakeClusterClientForUpdateManifest) Pods() PodClient                  { return nil }
func (f *fakeClusterClientForUpdateManifest) Authorization() AuthorizationClient {
	return nil
}
func (f *fakeClusterClientForUpdateManifest) SSA() DynamicSSAClient      { return nil }
func (f *fakeClusterClientForUpdateManifest) KubeVirt() KubeVirtCRClient { return nil }

type fakeVMClientForUpdateManifest struct {
	vm *kubevirtv1.VirtualMachine
}

func (f *fakeVMClientForUpdateManifest) Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachine, error) {
	return f.vm.DeepCopy(), nil
}
func (f *fakeVMClientForUpdateManifest) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineList, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeVMClientForUpdateManifest) Patch(ctx context.Context, namespace, name string, pt types.PatchType, data []byte, opts k8smetav1.PatchOptions, subresources ...string) (*kubevirtv1.VirtualMachine, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeVMClientForUpdateManifest) Delete(ctx context.Context, namespace, name string, opts k8smetav1.DeleteOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeVMClientForUpdateManifest) Start(ctx context.Context, namespace, name string, opts *kubevirtv1.StartOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeVMClientForUpdateManifest) Stop(ctx context.Context, namespace, name string, opts *kubevirtv1.StopOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeVMClientForUpdateManifest) Restart(ctx context.Context, namespace, name string, opts *kubevirtv1.RestartOptions) error {
	return fmt.Errorf("not implemented")
}

type capturingPatchClusterClient struct {
	vm *capturingPatchVMClient
}

func (f *capturingPatchClusterClient) VM() VirtualMachineClient { return f.vm }
func (f *capturingPatchClusterClient) VMI() VirtualMachineInstanceClient {
	return nil
}
func (f *capturingPatchClusterClient) DataVolume() DataVolumeClient { return nil }
func (f *capturingPatchClusterClient) StorageProfile() StorageProfileClient {
	return nil
}
func (f *capturingPatchClusterClient) PVC() PersistentVolumeClaimClient { return nil }
func (f *capturingPatchClusterClient) StorageClass() StorageClassClient { return nil }
func (f *capturingPatchClusterClient) Events() EventClient              { return nil }
func (f *capturingPatchClusterClient) Namespaces() NamespaceClient      { return nil }
func (f *capturingPatchClusterClient) Nodes() NodeClient                { return nil }
func (f *capturingPatchClusterClient) Pods() PodClient                  { return nil }
func (f *capturingPatchClusterClient) Authorization() AuthorizationClient {
	return nil
}
func (f *capturingPatchClusterClient) SSA() DynamicSSAClient      { return nil }
func (f *capturingPatchClusterClient) KubeVirt() KubeVirtCRClient { return nil }

type capturingPatchVMClient struct {
	result        *kubevirtv1.VirtualMachine
	err           error
	startErr      error
	stopErr       error
	restartErr    error
	patchCalled   bool
	namespace     string
	name          string
	patchType     types.PatchType
	patchPayload  []byte
	patchPayloads [][]byte
	patchOpts     k8smetav1.PatchOptions
	startCalls    int
	stopCalls     int
	restartCalls  int
}

func (f *capturingPatchVMClient) Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachine, error) {
	if f.result == nil {
		return nil, fmt.Errorf("not implemented")
	}
	return f.result.DeepCopy(), nil
}
func (f *capturingPatchVMClient) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineList, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *capturingPatchVMClient) Patch(ctx context.Context, namespace, name string, pt types.PatchType, data []byte, opts k8smetav1.PatchOptions, subresources ...string) (*kubevirtv1.VirtualMachine, error) {
	f.patchCalled = true
	f.namespace = namespace
	f.name = name
	f.patchType = pt
	f.patchPayload = append([]byte(nil), data...)
	f.patchPayloads = append(f.patchPayloads, append([]byte(nil), data...))
	f.patchOpts = opts
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		f.result = &kubevirtv1.VirtualMachine{
			ObjectMeta: k8smetav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
	}
	return f.result.DeepCopy(), nil
}
func (f *capturingPatchVMClient) Delete(ctx context.Context, namespace, name string, opts k8smetav1.DeleteOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *capturingPatchVMClient) Start(ctx context.Context, namespace, name string, opts *kubevirtv1.StartOptions) error {
	f.startCalls++
	return f.startErr
}
func (f *capturingPatchVMClient) Stop(ctx context.Context, namespace, name string, opts *kubevirtv1.StopOptions) error {
	f.stopCalls++
	return f.stopErr
}
func (f *capturingPatchVMClient) Restart(ctx context.Context, namespace, name string, opts *kubevirtv1.RestartOptions) error {
	f.restartCalls++
	return f.restartErr
}

func assertRunStrategyPatch(t *testing.T, vmClient *capturingPatchVMClient, strategy kubevirtv1.VirtualMachineRunStrategy) {
	t.Helper()
	if !vmClient.patchCalled {
		t.Fatal("Patch() was not called")
	}
	if vmClient.patchType != types.MergePatchType {
		t.Fatalf("Patch() type = %q, want %q", vmClient.patchType, types.MergePatchType)
	}
	if vmClient.namespace != "team-a" || vmClient.name != "vm-a" {
		t.Fatalf("Patch() target = %s/%s, want team-a/vm-a", vmClient.namespace, vmClient.name)
	}
	assertRunStrategyPatchPayload(t, vmClient.patchPayload, strategy)
}

func assertRunStrategyPatchPayload(t *testing.T, payload []byte, strategy kubevirtv1.VirtualMachineRunStrategy) {
	t.Helper()
	var patch struct {
		Spec struct {
			RunStrategy string `json:"runStrategy"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(payload, &patch); err != nil {
		t.Fatalf("Patch() payload is not valid JSON: %v", err)
	}
	if patch.Spec.RunStrategy != string(strategy) {
		t.Fatalf("Patch() runStrategy = %q, want %q", patch.Spec.RunStrategy, strategy)
	}
}

type createGuardGetResult struct {
	vm  *kubevirtv1.VirtualMachine
	err error
}

type createGuardVMClient struct {
	gets     []createGuardGetResult
	getCalls int
}

func (f *createGuardVMClient) Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachine, error) {
	if f.getCalls >= len(f.gets) {
		return nil, fmt.Errorf("unexpected get %s/%s", namespace, name)
	}
	result := f.gets[f.getCalls]
	f.getCalls++
	if result.err != nil {
		return nil, result.err
	}
	return result.vm.DeepCopy(), nil
}
func (f *createGuardVMClient) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineList, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *createGuardVMClient) Patch(ctx context.Context, namespace, name string, pt types.PatchType, data []byte, opts k8smetav1.PatchOptions, subresources ...string) (*kubevirtv1.VirtualMachine, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *createGuardVMClient) Delete(ctx context.Context, namespace, name string, opts k8smetav1.DeleteOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *createGuardVMClient) Start(ctx context.Context, namespace, name string, opts *kubevirtv1.StartOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *createGuardVMClient) Stop(ctx context.Context, namespace, name string, opts *kubevirtv1.StopOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *createGuardVMClient) Restart(ctx context.Context, namespace, name string, opts *kubevirtv1.RestartOptions) error {
	return fmt.Errorf("not implemented")
}

type createGuardSSAClient struct {
	applyCalled bool
}

func (f *createGuardSSAClient) ApplyYAML(ctx context.Context, namespace string, yamlData []byte) (*unstructured.Unstructured, error) {
	f.applyCalled = true
	name, err := extractNameFromYAML(yamlData)
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj, nil
}
func (f *createGuardSSAClient) ApplyClusterScopedYAML(ctx context.Context, gvr schema.GroupVersionResource, yamlData []byte) (*unstructured.Unstructured, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *createGuardSSAClient) DryRunApplyYAML(ctx context.Context, namespace string, yamlData []byte) error {
	return fmt.Errorf("not implemented")
}

func kubevirtVMWithEventID(name, namespace, eventID string) *kubevirtv1.VirtualMachine {
	return &kubevirtv1.VirtualMachine{
		ObjectMeta: k8smetav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				domain.ShepherdEventIDLabel: eventID,
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: k8smetav1.ObjectMeta{
					Labels: map[string]string{
						domain.ShepherdEventIDLabel: eventID,
					},
				},
			},
		},
	}
}

type timeoutProbeClusterClient struct {
	vm    VirtualMachineClient
	vmi   VirtualMachineInstanceClient
	nodes NodeClient
	ssa   DynamicSSAClient
}

func (f *timeoutProbeClusterClient) VM() VirtualMachineClient {
	return f.vm
}
func (f *timeoutProbeClusterClient) VMI() VirtualMachineInstanceClient {
	return f.vmi
}
func (f *timeoutProbeClusterClient) DataVolume() DataVolumeClient { return nil }
func (f *timeoutProbeClusterClient) StorageProfile() StorageProfileClient {
	return nil
}
func (f *timeoutProbeClusterClient) PVC() PersistentVolumeClaimClient { return nil }
func (f *timeoutProbeClusterClient) StorageClass() StorageClassClient { return nil }
func (f *timeoutProbeClusterClient) Events() EventClient              { return nil }
func (f *timeoutProbeClusterClient) Namespaces() NamespaceClient      { return nil }
func (f *timeoutProbeClusterClient) Nodes() NodeClient                { return f.nodes }
func (f *timeoutProbeClusterClient) Pods() PodClient                  { return nil }
func (f *timeoutProbeClusterClient) Authorization() AuthorizationClient {
	return nil
}
func (f *timeoutProbeClusterClient) SSA() DynamicSSAClient      { return f.ssa }
func (f *timeoutProbeClusterClient) KubeVirt() KubeVirtCRClient { return nil }

type catalogProbeClusterClient struct {
	instanceTypes      *instancetypev1beta1.VirtualMachineInstancetypeList
	clusterTypes       *instancetypev1beta1.VirtualMachineClusterInstancetypeList
	preferences        *instancetypev1beta1.VirtualMachinePreferenceList
	clusterPreferences *instancetypev1beta1.VirtualMachineClusterPreferenceList

	instanceTypesCtx       context.Context
	clusterTypesCtx        context.Context
	preferencesCtx         context.Context
	clusterPreferencesCtx  context.Context
	instanceTypesNamespace string
	preferencesNamespace   string
}

func (f *catalogProbeClusterClient) VM() VirtualMachineClient          { return nil }
func (f *catalogProbeClusterClient) VMI() VirtualMachineInstanceClient { return nil }
func (f *catalogProbeClusterClient) DataVolume() DataVolumeClient      { return nil }
func (f *catalogProbeClusterClient) StorageProfile() StorageProfileClient {
	return nil
}
func (f *catalogProbeClusterClient) PVC() PersistentVolumeClaimClient { return nil }
func (f *catalogProbeClusterClient) StorageClass() StorageClassClient { return nil }
func (f *catalogProbeClusterClient) Events() EventClient              { return nil }
func (f *catalogProbeClusterClient) Namespaces() NamespaceClient      { return nil }
func (f *catalogProbeClusterClient) Nodes() NodeClient                { return nil }
func (f *catalogProbeClusterClient) Pods() PodClient                  { return nil }
func (f *catalogProbeClusterClient) Authorization() AuthorizationClient {
	return nil
}
func (f *catalogProbeClusterClient) SSA() DynamicSSAClient      { return nil }
func (f *catalogProbeClusterClient) KubeVirt() KubeVirtCRClient { return nil }

func (f *catalogProbeClusterClient) ListInstanceTypes(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*instancetypev1beta1.VirtualMachineInstancetypeList, error) {
	f.instanceTypesCtx = ctx
	f.instanceTypesNamespace = namespace
	if f.instanceTypes == nil {
		return nil, fmt.Errorf("not implemented")
	}
	return f.instanceTypes.DeepCopy(), nil
}

func (f *catalogProbeClusterClient) ListClusterInstanceTypes(ctx context.Context, opts k8smetav1.ListOptions) (*instancetypev1beta1.VirtualMachineClusterInstancetypeList, error) {
	f.clusterTypesCtx = ctx
	if f.clusterTypes == nil {
		return nil, fmt.Errorf("not implemented")
	}
	return f.clusterTypes.DeepCopy(), nil
}

func (f *catalogProbeClusterClient) ListPreferences(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*instancetypev1beta1.VirtualMachinePreferenceList, error) {
	f.preferencesCtx = ctx
	f.preferencesNamespace = namespace
	if f.preferences == nil {
		return nil, fmt.Errorf("not implemented")
	}
	return f.preferences.DeepCopy(), nil
}

func (f *catalogProbeClusterClient) ListClusterPreferences(ctx context.Context, opts k8smetav1.ListOptions) (*instancetypev1beta1.VirtualMachineClusterPreferenceList, error) {
	f.clusterPreferencesCtx = ctx
	if f.clusterPreferences == nil {
		return nil, fmt.Errorf("not implemented")
	}
	return f.clusterPreferences.DeepCopy(), nil
}

type timeoutProbeVMClient struct {
	getCtx  context.Context
	listCtx context.Context
	vm      *kubevirtv1.VirtualMachine
	list    *kubevirtv1.VirtualMachineList
}

func (f *timeoutProbeVMClient) Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachine, error) {
	f.getCtx = ctx
	if f.vm == nil {
		return nil, fmt.Errorf("not implemented")
	}
	return f.vm.DeepCopy(), nil
}
func (f *timeoutProbeVMClient) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineList, error) {
	f.listCtx = ctx
	if f.list == nil {
		return nil, fmt.Errorf("not implemented")
	}
	return f.list.DeepCopy(), nil
}
func (f *timeoutProbeVMClient) Patch(ctx context.Context, namespace, name string, pt types.PatchType, data []byte, opts k8smetav1.PatchOptions, subresources ...string) (*kubevirtv1.VirtualMachine, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *timeoutProbeVMClient) Delete(ctx context.Context, namespace, name string, opts k8smetav1.DeleteOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *timeoutProbeVMClient) Start(ctx context.Context, namespace, name string, opts *kubevirtv1.StartOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *timeoutProbeVMClient) Stop(ctx context.Context, namespace, name string, opts *kubevirtv1.StopOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *timeoutProbeVMClient) Restart(ctx context.Context, namespace, name string, opts *kubevirtv1.RestartOptions) error {
	return fmt.Errorf("not implemented")
}

type timeoutProbeVMIClient struct {
	getCtx  context.Context
	listCtx context.Context
	vmi     *kubevirtv1.VirtualMachineInstance
	list    *kubevirtv1.VirtualMachineInstanceList
}

func (f *timeoutProbeVMIClient) Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachineInstance, error) {
	f.getCtx = ctx
	if f.vmi == nil {
		return nil, nil
	}
	return f.vmi.DeepCopy(), nil
}
func (f *timeoutProbeVMIClient) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineInstanceList, error) {
	f.listCtx = ctx
	if f.list == nil {
		return nil, nil
	}
	return f.list.DeepCopy(), nil
}
func (f *timeoutProbeVMIClient) Pause(ctx context.Context, namespace, name string, opts *kubevirtv1.PauseOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *timeoutProbeVMIClient) Unpause(ctx context.Context, namespace, name string, opts *kubevirtv1.UnpauseOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *timeoutProbeVMIClient) VNC(namespace, name string, preserveSession bool) (net.Conn, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *timeoutProbeVMIClient) SerialConsole(namespace, name string, connectionTimeout time.Duration) (net.Conn, error) {
	return nil, fmt.Errorf("not implemented")
}

type timeoutProbeNodeClient struct {
	getCtx context.Context
	node   *corev1.Node
}

func (f *timeoutProbeNodeClient) Get(ctx context.Context, name string, opts k8smetav1.GetOptions) (*corev1.Node, error) {
	f.getCtx = ctx
	if f.node == nil {
		return nil, fmt.Errorf("not implemented")
	}
	return f.node.DeepCopy(), nil
}
func (f *timeoutProbeNodeClient) List(ctx context.Context, opts k8smetav1.ListOptions) (*corev1.NodeList, error) {
	return nil, fmt.Errorf("not implemented")
}

type timeoutProbeSSAClient struct {
	dryRunCtx context.Context
}

func (f *timeoutProbeSSAClient) ApplyYAML(ctx context.Context, namespace string, yamlData []byte) (*unstructured.Unstructured, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *timeoutProbeSSAClient) ApplyClusterScopedYAML(ctx context.Context, gvr schema.GroupVersionResource, yamlData []byte) (*unstructured.Unstructured, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *timeoutProbeSSAClient) DryRunApplyYAML(ctx context.Context, namespace string, yamlData []byte) error {
	f.dryRunCtx = ctx
	return nil
}

func ptrRunStrategy(value kubevirtv1.VirtualMachineRunStrategy) *kubevirtv1.VirtualMachineRunStrategy {
	return &value
}

func TestKubevirtSSAApplier_DecodeAndMarshal_EmptyYAML(t *testing.T) {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
	applier := NewKubevirtSSAApplier(dynClient)

	_, _, err := applier.decodeAndMarshal([]byte(""))
	if err == nil {
		t.Fatalf("expected error for empty YAML, got nil")
	}
}

func TestValidateYAMLResourceHalfSteps_AcceptsStandardValues(t *testing.T) {
	if err := validateYAMLResourceHalfSteps([]byte(validVMYAML)); err != nil {
		t.Fatalf("expected standard values to pass, got: %v", err)
	}
}

func TestValidateYAMLResourceHalfSteps_RejectsNonStandardCPU(t *testing.T) {
	invalid := strings.Replace(validVMYAML, `cpu: "4"`, `cpu: "1300m"`, 1)
	err := validateYAMLResourceHalfSteps([]byte(invalid))
	if err == nil {
		t.Fatalf("expected non-standard cpu to fail")
	}
	if !strings.Contains(err.Error(), "500m increments") {
		t.Fatalf("expected cpu half-step error, got: %v", err)
	}
}

func TestMapStorageProfile_PrefersStatusOverSpec(t *testing.T) {
	filesystem := corev1.PersistentVolumeFilesystem
	block := corev1.PersistentVolumeBlock
	rwo := corev1.ReadWriteOnce
	rwx := corev1.ReadWriteMany
	copyStrategy := cdiv1beta1.CDICloneStrategy("copy")
	snapshotStrategy := cdiv1beta1.CDICloneStrategy("snapshot")

	storageProfile := &cdiv1beta1.StorageProfile{}
	storageProfile.Name = "gold"
	storageProfile.Spec.CloneStrategy = &copyStrategy
	storageProfile.Spec.ClaimPropertySets = []cdiv1beta1.ClaimPropertySet{
		{AccessModes: []corev1.PersistentVolumeAccessMode{rwo}, VolumeMode: &filesystem},
	}
	storageProfile.Status.CloneStrategy = &snapshotStrategy
	storageProfile.Status.ClaimPropertySets = []cdiv1beta1.ClaimPropertySet{
		{AccessModes: []corev1.PersistentVolumeAccessMode{rwx, rwo}, VolumeMode: &block},
	}

	got := mapStorageProfile(storageProfile)
	if got.Name != "gold" {
		t.Fatalf("Name = %q, want %q", got.Name, "gold")
	}
	if got.CloneStrategy != "snapshot" {
		t.Fatalf("CloneStrategy = %q, want %q", got.CloneStrategy, "snapshot")
	}
	if got.DefaultVolumeMode != "Block" {
		t.Fatalf("DefaultVolumeMode = %q, want %q", got.DefaultVolumeMode, "Block")
	}
	if len(got.ClaimPropertySets) != 1 {
		t.Fatalf("ClaimPropertySets len = %d, want 1", len(got.ClaimPropertySets))
	}
	if got.ClaimPropertySets[0].VolumeMode != "Block" {
		t.Fatalf("ClaimPropertySets[0].VolumeMode = %q, want %q", got.ClaimPropertySets[0].VolumeMode, "Block")
	}
	if diff := cmp.Diff([]string{"ReadWriteMany", "ReadWriteOnce"}, got.ClaimPropertySets[0].AccessModes); diff != "" {
		t.Fatalf("ClaimPropertySets[0].AccessModes mismatch (-want +got):\n%s", diff)
	}
}

func TestMapStorageProfile_FallsBackToSpecWhenStatusEmpty(t *testing.T) {
	filesystem := corev1.PersistentVolumeFilesystem
	rwo := corev1.ReadWriteOnce
	copyStrategy := cdiv1beta1.CDICloneStrategy("copy")

	storageProfile := &cdiv1beta1.StorageProfile{}
	storageProfile.Name = "slow"
	storageProfile.Spec.CloneStrategy = &copyStrategy
	storageProfile.Spec.ClaimPropertySets = []cdiv1beta1.ClaimPropertySet{
		{AccessModes: []corev1.PersistentVolumeAccessMode{rwo}, VolumeMode: &filesystem},
	}

	got := mapStorageProfile(storageProfile)
	if got.CloneStrategy != "copy" {
		t.Fatalf("CloneStrategy = %q, want %q", got.CloneStrategy, "copy")
	}
	if got.DefaultVolumeMode != "Filesystem" {
		t.Fatalf("DefaultVolumeMode = %q, want %q", got.DefaultVolumeMode, "Filesystem")
	}
	if diff := cmp.Diff([]string{"ReadWriteOnce"}, got.ClaimPropertySets[0].AccessModes); diff != "" {
		t.Fatalf("ClaimPropertySets[0].AccessModes mismatch (-want +got):\n%s", diff)
	}
}

func TestValidateYAMLResourceHalfSteps_RejectsNonStandardMemory(t *testing.T) {
	invalid := strings.Replace(validVMYAML, `memory: "8Gi"`, `memory: "1300Mi"`, 1)
	err := validateYAMLResourceHalfSteps([]byte(invalid))
	if err == nil {
		t.Fatalf("expected non-standard memory to fail")
	}
	if !strings.Contains(err.Error(), "512Mi increments") {
		t.Fatalf("expected memory half-step error, got: %v", err)
	}
}
