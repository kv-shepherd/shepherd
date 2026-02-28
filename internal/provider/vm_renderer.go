package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// vmYAMLTemplate is the canonical KubeVirt VirtualMachine YAML template.
//
// Architecture per ADR-0011: Backend is a "YAML porter". The VM spec is rendered
// as YAML, never assembled as Go typed structs. This template is the single
// source of truth for the VM structure.
//
// Resource units:
//   - CPU: Kubernetes millicores string (e.g. "500m" for 0.5 cores, "2" for 2 cores)
//   - Memory: Kubernetes Gi/Mi string (e.g. "512Mi" for 0.5Gi, "8Gi" for 8Gi)
//
// All values must be in 0.5-step increments (0.5, 1.0, 1.5, 2.0, ...).
const vmYAMLTemplate = `apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: "{{.Name}}"
  namespace: "{{.Namespace}}"
  {{- if .Labels}}
  labels:
    {{- range $k, $v := .Labels}}
    {{$k}}: "{{$v}}"
    {{- end}}
  {{- end}}
spec:
  runStrategy: Always
  template:
    metadata:
      {{- if .Labels}}
      labels:
        {{- range $k, $v := .Labels}}
        {{$k}}: "{{$v}}"
        {{- end}}
      {{- end}}
    spec:
      domain:
        cpu:
          cores: {{.CPUCores}}
        resources:
          requests:
            cpu: "{{.CPURequest}}"
            memory: "{{.MemoryRequest}}"
          limits:
            cpu: "{{.CPULimit}}"
            memory: "{{.MemoryLimit}}"
        devices:
          disks:
            - name: rootdisk
              disk:
                bus: virtio
            {{- if gt .DiskGB 0}}
            - name: datadisk
              disk:
                bus: virtio
            {{- end}}
            {{- if .CloudInit}}
            - name: cloudinitdisk
              disk:
                bus: virtio
            {{- end}}
      volumes:
        {{- if .IsPVC}}
        - name: rootdisk
          persistentVolumeClaim:
            claimName: "{{.PVCClaimName}}"
        {{- else}}
        - name: rootdisk
          containerDisk:
            image: "{{.Image}}"
        {{- end}}
        {{- if gt .DiskGB 0}}
        - name: datadisk
          emptyDisk:
            capacity: "{{.DiskGB}}Gi"
        {{- end}}
        {{- if .CloudInit}}
        - name: cloudinitdisk
          cloudInitNoCloud:
            userData: {{ printf "%q" .CloudInit }}
        {{- end}}
`

// vmTemplateData holds pre-computed values for the VM YAML template.
type vmTemplateData struct {
	Name          string
	Namespace     string
	Labels        map[string]string
	CPUCores      int    // integer core count for spec.domain.cpu.cores
	CPULimit      string // K8s quantity string for limits (e.g. "2" or "500m")
	CPURequest    string // K8s quantity string for requests
	MemoryLimit   string // K8s Gi/Mi string (e.g. "8Gi" or "512Mi")
	MemoryRequest string // K8s Gi/Mi string
	DiskGB        int
	Image         string
	CloudInit     string
	IsPVC         bool
	PVCClaimName  string
}

// RenderVMSpecToYAML converts a VMRenderInput into a KubeVirt VirtualMachine YAML string.
//
// This is the "YAML porter" implementation required by ADR-0011.
// The rendered YAML is consumed by DynamicSSAClient.ApplyYAML().
//
// Resource granularity: CPU and Memory must be in 0.5-step increments.
//   - CPU: 0.5, 1.0, 1.5, 2.0, ... (in cores)
//   - Memory: 0.5, 1.0, 1.5, 2.0, ... (in Gi)
//
// SpecOverrides (ADR-0018 Hybrid Model) are applied as deep-merge patches into
// the rendered YAML after template execution. Override paths are validated to
// start with "spec." prefix to prevent overwriting metadata or apiVersion.
//
// Transition: When ADR-0007 user-managed templates are implemented, callers should
// use the template rendering pipeline instead and set spec.RenderedYAML directly.
func RenderVMSpecToYAML(namespace string, spec *VMRenderInput) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("render vm yaml: spec is nil")
	}
	if spec.Name == "" {
		return "", fmt.Errorf("render vm yaml: name is required")
	}
	if spec.CPUCores <= 0 {
		return "", fmt.Errorf("render vm yaml: cpu must be > 0")
	}
	if spec.MemoryGi <= 0 {
		return "", fmt.Errorf("render vm yaml: memory must be > 0")
	}
	if spec.Image == "" {
		return "", fmt.Errorf("render vm yaml: image is required")
	}

	// Validate 0.5-step granularity.
	if !isValidHalfStep(spec.CPUCores) {
		return "", fmt.Errorf("render vm yaml: cpu %.1f is not a valid 0.5-step value (allowed: 0.5, 1.0, 1.5, ...)", spec.CPUCores)
	}
	if !isValidHalfStep(spec.MemoryGi) {
		return "", fmt.Errorf("render vm yaml: memory %.1fGi is not a valid 0.5-step value (allowed: 0.5, 1.0, 1.5, ...)", spec.MemoryGi)
	}
	if spec.CPURequest > 0 && !isValidHalfStep(spec.CPURequest) {
		return "", fmt.Errorf("render vm yaml: cpu_request %.1f is not a valid 0.5-step value", spec.CPURequest)
	}
	if spec.MemoryRequestGi > 0 && !isValidHalfStep(spec.MemoryRequestGi) {
		return "", fmt.Errorf("render vm yaml: memory_request %.1fGi is not a valid 0.5-step value", spec.MemoryRequestGi)
	}

	// Determine image source type.
	isPVC := false
	pvcClaimName := ""
	image := spec.Image
	if strings.HasPrefix(spec.Image, "pvc:") {
		isPVC = true
		pvcClaimName = strings.TrimPrefix(spec.Image, "pvc:")
		// Handle "pvc:namespace/name" format — extract just the name part.
		if idx := strings.LastIndex(pvcClaimName, "/"); idx >= 0 {
			pvcClaimName = pvcClaimName[idx+1:]
		}
		image = "" // not used for PVC
	}

	// Compute CPU strings.
	cpuLimitCores := spec.CPUCores
	cpuRequestCores := cpuLimitCores
	if spec.CPURequest > 0 {
		cpuRequestCores = spec.CPURequest
	}

	// Compute memory strings.
	memoryLimit := formatGi(spec.MemoryGi)
	memoryRequest := memoryLimit
	if spec.MemoryRequestGi > 0 {
		memoryRequest = formatGi(spec.MemoryRequestGi)
	}

	data := vmTemplateData{
		Name:          spec.Name,
		Namespace:     namespace,
		Labels:        spec.Labels,
		CPUCores:      cpuCoresForTopology(cpuLimitCores),
		CPULimit:      formatCPU(cpuLimitCores),
		CPURequest:    formatCPU(cpuRequestCores),
		MemoryLimit:   memoryLimit,
		MemoryRequest: memoryRequest,
		DiskGB:        spec.DiskGB,
		Image:         image,
		CloudInit:     spec.CloudInit,
		IsPVC:         isPVC,
		PVCClaimName:  pvcClaimName,
	}

	tmpl, err := template.New("vm").Parse(vmYAMLTemplate)
	if err != nil {
		return "", fmt.Errorf("parse vm template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute vm template: %w", err)
	}

	// If no SpecOverrides, return the template-rendered YAML directly.
	if len(spec.SpecOverrides) == 0 {
		return buf.String(), nil
	}

	// Validate SpecOverrides paths before applying.
	if err := validateOverridePaths(spec.SpecOverrides); err != nil {
		return "", err
	}
	// Enforce CPU/Memory standard step for resource overrides as well.
	if err := validateOverrideResourceSteps(spec.SpecOverrides); err != nil {
		return "", err
	}

	// Apply SpecOverrides as deep-merge patches into the rendered YAML.
	return applySpecOverridesToYAML(buf.Bytes(), spec.SpecOverrides)
}

// validateOverridePaths ensures all SpecOverrides keys use the "spec.*" prefix,
// consistent with ADR-0018 §4 and service.ValidateSpecOverrides().
// This prevents overwriting metadata, apiVersion, kind, or other non-spec fields.
func validateOverridePaths(overrides map[string]interface{}) error {
	for key, rawValue := range overrides {
		path := strings.TrimSpace(key)
		if path == "" {
			continue
		}
		if path != "spec" && !strings.HasPrefix(path, "spec.") {
			return fmt.Errorf(
				"invalid spec_overrides path %q: must start with \"spec.\" prefix (ADR-0018); "+
					"overriding metadata, apiVersion, or kind is not allowed",
				key,
			)
		}
		// Guardrail: "spec" root override must be an object so we can deep-merge
		// leaf paths. A scalar at spec root would replace the whole spec object.
		if path == "spec" {
			if _, ok := rawValue.(map[string]interface{}); !ok {
				return fmt.Errorf(
					"invalid spec_overrides path %q: value must be an object for deep merge",
					key,
				)
			}
		}
	}
	return nil
}

var cpuResourcePaths = map[string]struct{}{
	"spec.template.spec.domain.resources.requests.cpu": {},
	"spec.template.spec.domain.resources.limits.cpu":   {},
	"spec.domain.resources.requests.cpu":               {},
	"spec.domain.resources.limits.cpu":                 {},
}

var memoryResourcePaths = map[string]struct{}{
	"spec.template.spec.domain.resources.requests.memory": {},
	"spec.template.spec.domain.resources.limits.memory":   {},
	"spec.domain.resources.requests.memory":               {},
	"spec.domain.resources.limits.memory":                 {},
}

const halfGiStepBytes = int64(512 * 1024 * 1024)

// validateOverrideResourceSteps enforces 0.5-step rules for resource quantities
// in SpecOverrides so that non-standard values cannot bypass template checks.
func validateOverrideResourceSteps(overrides map[string]interface{}) error {
	flat := flattenOverrideValues(overrides)
	for path, value := range flat {
		if _, ok := cpuResourcePaths[path]; ok {
			if err := validateCPUHalfStep(path, value); err != nil {
				return err
			}
			continue
		}
		if _, ok := memoryResourcePaths[path]; ok {
			if err := validateMemoryHalfStep(path, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func flattenOverrideValues(overrides map[string]interface{}) map[string]interface{} {
	flat := make(map[string]interface{})
	var walk func(path string, value interface{})
	walk = func(path string, value interface{}) {
		nested, ok := value.(map[string]interface{})
		if !ok {
			flat[path] = value
			return
		}
		for key, child := range nested {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			walk(path+"."+trimmed, child)
		}
	}

	for key, value := range overrides {
		path := strings.TrimSpace(key)
		if path == "" {
			continue
		}
		walk(path, value)
	}
	return flat
}

func validateCPUHalfStep(path string, value interface{}) error {
	milli, err := cpuValueToMilli(value)
	if err != nil {
		return fmt.Errorf("invalid spec_overrides value for %q: %w", path, err)
	}
	if milli <= 0 || milli%500 != 0 {
		return fmt.Errorf(
			"invalid spec_overrides value for %q: cpu must use 0.5-step values (500m increments), got %v",
			path, value,
		)
	}
	return nil
}

func cpuValueToMilli(value interface{}) (int64, error) {
	switch v := value.(type) {
	case string:
		q, err := resource.ParseQuantity(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("parse cpu quantity %q: %w", v, err)
		}
		return q.MilliValue(), nil
	case float64:
		return int64(math.Round(v * 1000)), nil
	case float32:
		return int64(math.Round(float64(v) * 1000)), nil
	case int:
		return int64(v) * 1000, nil
	case int64:
		return v * 1000, nil
	case int32:
		return int64(v) * 1000, nil
	case uint:
		return int64(v) * 1000, nil
	case uint64:
		return int64(v) * 1000, nil
	case uint32:
		return int64(v) * 1000, nil
	default:
		return 0, fmt.Errorf("unsupported cpu value type %T", value)
	}
}

func validateMemoryHalfStep(path string, value interface{}) error {
	bytes, err := memoryValueToBytes(value)
	if err != nil {
		return fmt.Errorf("invalid spec_overrides value for %q: %w", path, err)
	}
	if bytes <= 0 || bytes%halfGiStepBytes != 0 {
		return fmt.Errorf(
			"invalid spec_overrides value for %q: memory must use 0.5-step values (512Mi increments), got %v",
			path, value,
		)
	}
	return nil
}

func memoryValueToBytes(value interface{}) (int64, error) {
	switch v := value.(type) {
	case string:
		q, err := resource.ParseQuantity(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("parse memory quantity %q: %w", v, err)
		}
		return q.Value(), nil
	case float64:
		return int64(math.Round(v * 1024 * 1024)), nil // numeric override is interpreted as Mi
	case float32:
		return int64(math.Round(float64(v) * 1024 * 1024)), nil
	case int:
		return int64(v) * 1024 * 1024, nil // numeric override is interpreted as Mi
	case int64:
		return v * 1024 * 1024, nil
	case int32:
		return int64(v) * 1024 * 1024, nil
	case uint:
		return int64(v) * 1024 * 1024, nil
	case uint64:
		return int64(v) * 1024 * 1024, nil
	case uint32:
		return int64(v) * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("unsupported memory value type %T (use Kubernetes quantity string, e.g. \"1536Mi\")", value)
	}
}

// applySpecOverridesToYAML decodes YAML into an unstructured map, applies
// dot-notation path overrides, and re-encodes to YAML.
//
// Override paths follow the convention from ADR-0018:
//
//	"spec.template.spec.domain.cpu.dedicatedCpuPlacement": true
//	"spec.template.spec.domain.memory.hugepages.pageSize": "2Mi"
func applySpecOverridesToYAML(yamlData []byte, overrides map[string]interface{}) (string, error) {
	obj := &unstructured.Unstructured{}
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlData), 4096)
	if err := decoder.Decode(obj); err != nil {
		return "", fmt.Errorf("decode yaml for spec overrides: %w", err)
	}

	// Normalize both dotted and nested override formats to leaf-path patches.
	// This avoids replacing whole parent objects such as "spec".
	flatOverrides := flattenOverrideValues(overrides)
	paths := make([]string, 0, len(flatOverrides))
	for path := range flatOverrides {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		value := flatOverrides[path]
		segments := strings.Split(path, ".")
		if len(segments) == 0 {
			continue
		}
		if err := unstructured.SetNestedField(obj.Object, value, segments...); err != nil {
			return "", fmt.Errorf("set spec override %q: %w", path, err)
		}
	}

	// Re-encode to YAML.
	jsonData, err := json.Marshal(obj.Object)
	if err != nil {
		return "", fmt.Errorf("marshal overridden spec to json: %w", err)
	}

	yamlOut, err := yaml.JSONToYAML(jsonData)
	if err != nil {
		return "", fmt.Errorf("convert overridden spec to yaml: %w", err)
	}

	return string(yamlOut), nil
}

// VMRenderInput contains the fields needed to render a VM YAML template.
// This is a projection of domain.VMSpec with all fields needed for rendering.
//
// Resource granularity: All CPU/Memory values must be in 0.5-step increments.
// Non-standard values (0.7, 1.2, etc.) are rejected at render time.
type VMRenderInput struct {
	Name      string
	CPUCores  float64 // CPU limit in cores (0.5 step: 0.5, 1.0, 1.5, ...)
	MemoryGi  float64 // Memory limit in Gi (0.5 step: 0.5, 1.0, 1.5, ...)
	DiskGB    int
	Image     string
	CloudInit string
	Labels    map[string]string
	// CPURequest is for overcommit: CPU request in cores (must be <= CPUCores).
	CPURequest float64
	// MemoryRequestGi is for overcommit: Memory request in Gi (must be <= MemoryGi).
	MemoryRequestGi float64
	// SpecOverrides carries advanced KubeVirt spec path/value overrides (ADR-0018 Hybrid Model).
	// Keys are dot-notation paths starting with "spec." prefix.
	// Applied as deep-merge patches after template rendering.
	SpecOverrides map[string]interface{}
}

// isValidHalfStep checks that a value is a multiple of 0.5 (0.5, 1.0, 1.5, ...).
// Non-standard values like 0.7, 1.2, 3.3 are rejected.
func isValidHalfStep(v float64) bool {
	// Multiply by 2: valid values become integers (1, 2, 3, 4, ...).
	doubled := v * 2
	return doubled > 0 && math.Abs(doubled-math.Round(doubled)) < 1e-9
}

// formatCPU converts CPU cores (float64) to Kubernetes CPU quantity string.
//   - Integer values: "1", "2", "4"
//   - Half values: "500m", "1500m", "2500m"
func formatCPU(cores float64) string {
	if cores == float64(int(cores)) {
		return fmt.Sprintf("%d", int(cores))
	}
	// Convert to millicores for sub-integer values.
	return fmt.Sprintf("%dm", int(cores*1000))
}

// cpuCoresForTopology converts a fractional CPU limit/request into the integer
// topology value required by spec.domain.cpu.cores.
// KubeVirt requires integer core topology; for 0.5-step resource quantities we
// round up to keep topology >= effective CPU quantity.
func cpuCoresForTopology(cores float64) int {
	v := int(math.Ceil(cores))
	if v < 1 {
		return 1
	}
	return v
}

// formatGi converts Gi (float64) to Kubernetes memory quantity string.
//   - Integer values: "1Gi", "2Gi", "8Gi"
//   - Half values: "512Mi", "1536Mi" (0.5Gi = 512Mi, 1.5Gi = 1536Mi)
func formatGi(gi float64) string {
	if gi == float64(int(gi)) {
		return fmt.Sprintf("%dGi", int(gi))
	}
	// Convert to Mi for sub-Gi values (1 Gi = 1024 Mi).
	mi := int(gi * 1024)
	return fmt.Sprintf("%dMi", mi)
}
