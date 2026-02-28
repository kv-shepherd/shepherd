package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
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
