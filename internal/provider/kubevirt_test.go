package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
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
            - name: rootdisk
              disk:
                bus: virtio
      volumes:
        - name: rootdisk
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
	copyStrategy := cdiv1beta1.CDICloneStrategy("copy")
	snapshotStrategy := cdiv1beta1.CDICloneStrategy("snapshot")

	storageProfile := &cdiv1beta1.StorageProfile{}
	storageProfile.Name = "gold"
	storageProfile.Spec.CloneStrategy = &copyStrategy
	storageProfile.Spec.ClaimPropertySets = []cdiv1beta1.ClaimPropertySet{
		{VolumeMode: &filesystem},
	}
	storageProfile.Status.CloneStrategy = &snapshotStrategy
	storageProfile.Status.ClaimPropertySets = []cdiv1beta1.ClaimPropertySet{
		{VolumeMode: &block},
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
}

func TestMapStorageProfile_FallsBackToSpecWhenStatusEmpty(t *testing.T) {
	filesystem := corev1.PersistentVolumeFilesystem
	copyStrategy := cdiv1beta1.CDICloneStrategy("copy")

	storageProfile := &cdiv1beta1.StorageProfile{}
	storageProfile.Name = "slow"
	storageProfile.Spec.CloneStrategy = &copyStrategy
	storageProfile.Spec.ClaimPropertySets = []cdiv1beta1.ClaimPropertySet{
		{VolumeMode: &filesystem},
	}

	got := mapStorageProfile(storageProfile)
	if got.CloneStrategy != "copy" {
		t.Fatalf("CloneStrategy = %q, want %q", got.CloneStrategy, "copy")
	}
	if got.DefaultVolumeMode != "Filesystem" {
		t.Fatalf("DefaultVolumeMode = %q, want %q", got.DefaultVolumeMode, "Filesystem")
	}
}

func TestGetDataVolume_MapsStatusFields(t *testing.T) {
	t.Parallel()

	provider := NewKubeVirtProvider(func(cluster string) (KubeVirtClusterClient, error) {
		if cluster != "cluster-a" {
			t.Fatalf("cluster = %q, want %q", cluster, "cluster-a")
		}
		return providerTestClusterClient{
			dataVolumeClient: providerTestDataVolumeClient{
				dataVolume: &cdiv1beta1.DataVolume{
					ObjectMeta: k8smetav1.ObjectMeta{
						Name:      "root-dv",
						Namespace: "team-a",
						UID:       "dv-uid",
					},
					Status: cdiv1beta1.DataVolumeStatus{
						ClaimName:    "root-pvc",
						Phase:        cdiv1beta1.Succeeded,
						Progress:     "100.0%",
						RestartCount: 2,
						Conditions: []cdiv1beta1.DataVolumeCondition{
							{
								Type:               cdiv1beta1.DataVolumeReady,
								Status:             corev1.ConditionTrue,
								Reason:             "ImportSucceeded",
								Message:            "ready",
								LastTransitionTime: k8smetav1.NewTime(time.Unix(1700000000, 0)),
							},
						},
					},
				},
			},
		}, nil
	}, time.Minute)

	got, err := provider.GetDataVolume(context.Background(), "cluster-a", "team-a", "root-dv")
	if err != nil {
		t.Fatalf("GetDataVolume() error = %v", err)
	}
	if got.Name != "root-dv" || got.Namespace != "team-a" || got.UID != "dv-uid" {
		t.Fatalf("GetDataVolume() identity = %+v", got)
	}
	if got.ClaimName != "root-pvc" || got.Phase != "Succeeded" || got.Progress != "100.0%" || got.RestartCount != 2 {
		t.Fatalf("GetDataVolume() status = %+v", got)
	}
	if len(got.Conditions) != 1 || got.Conditions[0].Type != "Ready" || got.Conditions[0].Status != "True" {
		t.Fatalf("GetDataVolume() conditions = %+v", got.Conditions)
	}
}

func TestGetPersistentVolumeClaim_MapsCloneMetadata(t *testing.T) {
	t.Parallel()

	filesystem := corev1.PersistentVolumeFilesystem
	storageClass := "fast-sc"
	requested := resourceMustParse("20Gi")
	capacity := resourceMustParse("24Gi")

	provider := NewKubeVirtProvider(func(string) (KubeVirtClusterClient, error) {
		return providerTestClusterClient{
			pvcClient: providerTestPVCClient{
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: k8smetav1.ObjectMeta{
						Name:      "root-pvc",
						Namespace: "team-a",
						Annotations: map[string]string{
							"cdi.kubevirt.io/cloneType":           "copy",
							"cdi.kubevirt.io/clonePhase":          "Succeeded",
							"cdi.kubevirt.io/cloneFallbackReason": "storageClassMismatch",
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: &storageClass,
						VolumeMode:       &filesystem,
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: requested},
						},
					},
					Status: corev1.PersistentVolumeClaimStatus{
						Phase:    corev1.ClaimBound,
						Capacity: corev1.ResourceList{corev1.ResourceStorage: capacity},
					},
				},
			},
		}, nil
	}, time.Minute)

	got, err := provider.GetPersistentVolumeClaim(context.Background(), "cluster-a", "team-a", "root-pvc")
	if err != nil {
		t.Fatalf("GetPersistentVolumeClaim() error = %v", err)
	}
	if got.StorageClassName != "fast-sc" || got.VolumeMode != "Filesystem" || got.Phase != "Bound" {
		t.Fatalf("GetPersistentVolumeClaim() core mapping = %+v", got)
	}
	if got.RequestedStorageBytes <= 0 || got.CapacityBytes <= 0 {
		t.Fatalf("GetPersistentVolumeClaim() storage bytes = %+v", got)
	}
	if got.CloneType != "copy" || got.ClonePhase != "Succeeded" || got.CloneFallbackReason != "storageClassMismatch" {
		t.Fatalf("GetPersistentVolumeClaim() clone metadata = %+v", got)
	}
}

func TestListEventsForObject_MapsEventTimestamps(t *testing.T) {
	t.Parallel()

	first := k8smetav1.NewTime(time.Unix(1700000100, 0))
	last := k8smetav1.NewTime(time.Unix(1700000200, 0))

	provider := NewKubeVirtProvider(func(string) (KubeVirtClusterClient, error) {
		return providerTestClusterClient{
			eventClient: providerTestEventClient{
				list: &corev1.EventList{
					Items: []corev1.Event{
						{
							Type:           "Warning",
							Reason:         "CloneFailed",
							Message:        "quota denied",
							Count:          3,
							FirstTimestamp: first,
							LastTimestamp:  last,
						},
					},
				},
			},
		}, nil
	}, time.Minute)

	got, err := provider.ListEventsForObject(context.Background(), "cluster-a", domain.ObjectReference{
		Kind:      "DataVolume",
		Name:      "root-dv",
		Namespace: "team-a",
	})
	if err != nil {
		t.Fatalf("ListEventsForObject() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListEventsForObject() len = %d, want 1", len(got))
	}
	if got[0].Type != "Warning" || got[0].Reason != "CloneFailed" || got[0].Message != "quota denied" || got[0].Count != 3 {
		t.Fatalf("ListEventsForObject() event = %+v", got[0])
	}
	if !got[0].FirstObserved.Equal(first.Time) || !got[0].LastObserved.Equal(last.Time) {
		t.Fatalf("ListEventsForObject() timestamps = %+v", got[0])
	}
}

func TestGetStorageClass_MapsAllowVolumeExpansion(t *testing.T) {
	t.Parallel()

	allowExpansion := true
	provider := NewKubeVirtProvider(func(string) (KubeVirtClusterClient, error) {
		return providerTestClusterClient{
			storageClassClient: providerTestStorageClassClient{
				storageClass: &storagev1.StorageClass{
					ObjectMeta:           k8smetav1.ObjectMeta{Name: "fast-sc"},
					AllowVolumeExpansion: &allowExpansion,
				},
			},
		}, nil
	}, time.Minute)

	got, err := provider.GetStorageClass(context.Background(), "cluster-a", "fast-sc")
	if err != nil {
		t.Fatalf("GetStorageClass() error = %v", err)
	}
	if got.Name != "fast-sc" || !got.AllowVolumeExpansion {
		t.Fatalf("GetStorageClass() = %+v", got)
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

type providerTestClusterClient struct {
	vmClient             VirtualMachineClient
	vmiClient            VirtualMachineInstanceClient
	dataVolumeClient     DataVolumeClient
	storageProfileClient StorageProfileClient
	pvcClient            PersistentVolumeClaimClient
	storageClassClient   StorageClassClient
	eventClient          EventClient
	ssaClient            DynamicSSAClient
	kvCRClient           KubeVirtCRClient
}

func (c providerTestClusterClient) VM() VirtualMachineClient          { return c.vmClient }
func (c providerTestClusterClient) VMI() VirtualMachineInstanceClient { return c.vmiClient }
func (c providerTestClusterClient) DataVolume() DataVolumeClient      { return c.dataVolumeClient }
func (c providerTestClusterClient) StorageProfile() StorageProfileClient {
	return c.storageProfileClient
}
func (c providerTestClusterClient) PVC() PersistentVolumeClaimClient { return c.pvcClient }
func (c providerTestClusterClient) StorageClass() StorageClassClient { return c.storageClassClient }
func (c providerTestClusterClient) Events() EventClient              { return c.eventClient }
func (c providerTestClusterClient) SSA() DynamicSSAClient            { return c.ssaClient }
func (c providerTestClusterClient) KubeVirt() KubeVirtCRClient       { return c.kvCRClient }

type providerTestDataVolumeClient struct{ dataVolume *cdiv1beta1.DataVolume }

func (c providerTestDataVolumeClient) Get(_ context.Context, _, _ string, _ k8smetav1.GetOptions) (*cdiv1beta1.DataVolume, error) {
	return c.dataVolume, nil
}
func (c providerTestDataVolumeClient) List(_ context.Context, _ string, _ k8smetav1.ListOptions) (*cdiv1beta1.DataVolumeList, error) {
	return &cdiv1beta1.DataVolumeList{}, nil
}

type providerTestPVCClient struct{ pvc *corev1.PersistentVolumeClaim }

func (c providerTestPVCClient) Get(_ context.Context, _, _ string, _ k8smetav1.GetOptions) (*corev1.PersistentVolumeClaim, error) {
	return c.pvc, nil
}

type providerTestStorageClassClient struct{ storageClass *storagev1.StorageClass }

func (c providerTestStorageClassClient) Get(_ context.Context, _ string, _ k8smetav1.GetOptions) (*storagev1.StorageClass, error) {
	return c.storageClass, nil
}

type providerTestEventClient struct{ list *corev1.EventList }

func (c providerTestEventClient) List(_ context.Context, _ string, _ k8smetav1.ListOptions) (*corev1.EventList, error) {
	return c.list, nil
}

func resourceMustParse(value string) resource.Quantity {
	q, err := resource.ParseQuantity(value)
	if err != nil {
		panic(err)
	}
	return q
}
