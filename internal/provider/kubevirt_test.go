package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	kubevirtv1 "kubevirt.io/api/core/v1"
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
	result       *kubevirtv1.VirtualMachine
	err          error
	patchCalled  bool
	namespace    string
	name         string
	patchType    types.PatchType
	patchPayload []byte
	patchOpts    k8smetav1.PatchOptions
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
	return fmt.Errorf("not implemented")
}
func (f *capturingPatchVMClient) Stop(ctx context.Context, namespace, name string, opts *kubevirtv1.StopOptions) error {
	return fmt.Errorf("not implemented")
}
func (f *capturingPatchVMClient) Restart(ctx context.Context, namespace, name string, opts *kubevirtv1.RestartOptions) error {
	return fmt.Errorf("not implemented")
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
