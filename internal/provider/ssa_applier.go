package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"
)

const (
	// FieldOwner identifies kubevirt-shepherd as the field manager for SSA.
	// Per ADR-0011: platform is Source of Truth; Force=true overwrites manual edits.
	FieldOwner = "kubevirt-shepherd"

	vmGroup    = "kubevirt.io"
	vmVersion  = "v1"
	vmResource = "virtualmachines"
)

// vmGVR is the GroupVersionResource for KubeVirt VirtualMachine.
// Uses string constants (not kubevirtv1.SchemeGroupVersion) to decouple from
// the kubevirt.io/api Go module version — stable since KubeVirt v0.x.
var vmGVR = schema.GroupVersionResource{
	Group:    vmGroup,
	Version:  vmVersion,
	Resource: vmResource,
}

// KubevirtSSAApplier submits VirtualMachine resources via dynamic client + SSA.
// Implements DynamicSSAClient.
//
// Architecture (ADR-0011):
//
//	DB Template YAML → text/template render → YAML string
//	                                           ↓
//	                          json.Marshal unstructured.Unstructured
//	                                           ↓
//	                  dynamic client Patch(types.ApplyPatchType)
//	                  FieldManager: "kubevirt-shepherd", Force: true
type KubevirtSSAApplier struct {
	dynamicClient dynamic.Interface
}

// Compile-time interface compliance check.
var _ DynamicSSAClient = (*KubevirtSSAApplier)(nil)

// NewKubevirtSSAApplier creates a new SSA Applier backed by the given dynamic client.
func NewKubevirtSSAApplier(dynamicClient dynamic.Interface) *KubevirtSSAApplier {
	return &KubevirtSSAApplier{dynamicClient: dynamicClient}
}

// ApplyYAML submits YAML bytes as an SSA Patch to Kubernetes.
//
// The method:
//  1. Decodes YAML into an Unstructured object (zero dependency on typed structs).
//  2. Marshals to JSON (required by types.ApplyPatchType).
//  3. Patches via dynamic client with FieldManager and Force=true.
//
// Force=true ensures kubevirt-shepherd owns all fields it declares, overwriting
// any conflicting field ownership (e.g., manual kubectl edits).
func (a *KubevirtSSAApplier) ApplyYAML(ctx context.Context, namespace string, yamlData []byte) (*unstructured.Unstructured, error) {
	obj, jsonData, err := a.decodeAndMarshal(yamlData)
	if err != nil {
		return nil, err
	}

	name := obj.GetName()
	if name == "" {
		return nil, fmt.Errorf("resource name is required in yaml")
	}

	result, err := a.dynamicClient.
		Resource(vmGVR).
		Namespace(namespace).
		Patch(ctx, name, types.ApplyPatchType, jsonData, k8smetav1.PatchOptions{
			FieldManager: FieldOwner,
			Force:        ptr.To(true),
		})
	if err != nil {
		return nil, fmt.Errorf("ssa apply %s/%s: %w", namespace, name, err)
	}

	return result, nil
}

// DryRunApplyYAML validates YAML via SSA DryRun without creating the resource.
// Used by ValidateSpec to leverage server-side validation (more authoritative
// than compile-time checks for external CRD fields).
func (a *KubevirtSSAApplier) DryRunApplyYAML(ctx context.Context, namespace string, yamlData []byte) error {
	obj, jsonData, err := a.decodeAndMarshal(yamlData)
	if err != nil {
		return err
	}

	name := obj.GetName()
	if name == "" {
		return fmt.Errorf("resource name is required in yaml")
	}

	_, err = a.dynamicClient.
		Resource(vmGVR).
		Namespace(namespace).
		Patch(ctx, name, types.ApplyPatchType, jsonData, k8smetav1.PatchOptions{
			FieldManager: FieldOwner,
			Force:        ptr.To(true),
			DryRun:       []string{k8smetav1.DryRunAll},
		})
	return err
}

// decodeAndMarshal converts YAML bytes into an Unstructured object and its JSON
// representation. This is a common step for both ApplyYAML and DryRunApplyYAML.
func (a *KubevirtSSAApplier) decodeAndMarshal(yamlData []byte) (*unstructured.Unstructured, []byte, error) {
	obj := &unstructured.Unstructured{}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlData), 4096)
	if err := decoder.Decode(obj); err != nil {
		return nil, nil, fmt.Errorf("decode yaml: %w", err)
	}

	jsonData, err := json.Marshal(obj.Object)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal to json: %w", err)
	}

	return obj, jsonData, nil
}
