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
  {{- if .HasRootDataVolume}}
  dataVolumeTemplates:
    - metadata:
        name: "{{.RootDataVolumeName}}"
      spec:
        {{- if .IsClonePVC}}
        source:
          pvc:
            {{- if .SourcePVCNamespace}}
            namespace: "{{.SourcePVCNamespace}}"
            {{- end}}
            name: "{{.SourcePVCName}}"
        {{- if .DVAccessModes}}
        pvc:
          accessModes:
          {{- range .DVAccessModes}}
          - {{.}}
          {{- end}}
          {{- if .RootDataVolumeStorageClass}}
          storageClassName: "{{.RootDataVolumeStorageClass}}"
          {{- end}}
          {{- if .DVVolumeMode}}
          volumeMode: {{.DVVolumeMode}}
          {{- end}}
          resources:
            requests:
              storage: "{{.RootDataVolumeSize}}"
        {{- else if or .RootDataVolumeStorageClass .RootDataVolumeSize}}
        storage:
          {{- if .RootDataVolumeStorageClass}}
          storageClassName: "{{.RootDataVolumeStorageClass}}"
          {{- end}}
          {{- if .RootDataVolumeSize}}
          resources:
            requests:
              storage: "{{.RootDataVolumeSize}}"
          {{- end}}
        {{- else}}
        storage: {}
        {{- end}}
        {{- else if .IsImportImage}}
        source:
          {{- if eq .ImportSourceKind "registry"}}
          registry:
            url: "{{.ImportSourceURL}}"
          {{- else if eq .ImportSourceKind "http"}}
          http:
            url: "{{.ImportSourceURL}}"
        {{- end}}
        {{- if .DVAccessModes}}
        pvc:
          accessModes:
          {{- range .DVAccessModes}}
          - {{.}}
          {{- end}}
          {{- if .RootDataVolumeStorageClass}}
          storageClassName: "{{.RootDataVolumeStorageClass}}"
          {{- end}}
          {{- if .DVVolumeMode}}
          volumeMode: {{.DVVolumeMode}}
          {{- end}}
          resources:
            requests:
              storage: "{{.RootDataVolumeSize}}"
        {{- else}}
        storage:
          {{- if .RootDataVolumeStorageClass}}
          storageClassName: "{{.RootDataVolumeStorageClass}}"
          {{- end}}
          resources:
            requests:
              storage: "{{.RootDataVolumeSize}}"
        {{- end}}
        {{- end}}
  {{- end}}
  template:
    {{- if .Labels}}
    metadata:
      labels:
        {{- range $k, $v := .Labels}}
        {{$k}}: "{{$v}}"
        {{- end}}
    {{- else}}
    metadata: {}
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
            - name: {{.RootDiskName}}
              disk:
                bus: virtio
            {{- if gt .DataDiskGB 0}}
            - name: {{.DataDiskName}}
              disk:
                bus: virtio
            {{- end}}
            {{- if .CloudInit}}
            - name: {{.CloudInitDiskName}}
              disk:
                bus: virtio
            {{- end}}
      volumes:
        {{- if .HasRootDataVolume}}
        - name: {{.RootDiskName}}
          dataVolume:
            name: "{{.RootDataVolumeName}}"
        {{- else}}
        - name: {{.RootDiskName}}
          containerDisk:
            image: "{{.Image}}"
        {{- end}}
        {{- if gt .DataDiskGB 0}}
        - name: {{.DataDiskName}}
          emptyDisk:
            capacity: "{{.DataDiskGB}}Gi"
        {{- end}}
        {{- if .CloudInit}}
        - name: {{.CloudInitDiskName}}
          cloudInitNoCloud:
            userData: {{ printf "%q" .CloudInit }}
        {{- end}}
`

// vmTemplateData holds pre-computed values for the VM YAML template.
//
// ADR-0018 Hybrid Model: This struct contains ONLY fields needed for
// structural template rendering. All VM behavior config (network, CPU model,
// lifecycle, device optimizations) is handled via spec_overrides deep-merge.
type vmTemplateData struct {
	Name                       string
	Namespace                  string
	Labels                     map[string]string
	CPUCores                   int    // integer core count for spec.domain.cpu.cores
	CPULimit                   string // K8s quantity string for limits (e.g. "2" or "500m")
	CPURequest                 string // K8s quantity string for requests
	MemoryLimit                string // K8s Gi/Mi string (e.g. "8Gi" or "512Mi")
	MemoryRequest              string // K8s Gi/Mi string
	DataDiskGB                 int
	Image                      string
	CloudInit                  string
	RootDiskName               string
	DataDiskName               string
	CloudInitDiskName          string
	HasRootDataVolume          bool
	IsClonePVC                 bool
	IsImportImage              bool
	SourcePVCName              string
	SourcePVCNamespace         string
	ImportSourceKind           string
	ImportSourceURL            string
	RootDataVolumeName         string
	RootDataVolumeSize         string
	RootDataVolumeStorageClass string

	// DataVolume storage mode options.
	// When DVAccessModes is non-empty, we use the CDI "pvc" format with explicit
	// accessModes/volumeMode instead of the "storage" format. This is needed for
	// storage backends that require specific modes (e.g. Ceph RBD → Block + RWX).
	// These are explicit fields (not in spec_overrides) because they change the
	// DV YAML structure (from storage: to pvc: format).
	DVAccessModes []string // e.g. ["ReadWriteMany"]
	DVVolumeMode  string   // e.g. "Block" or "Filesystem"
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
	hasRootDataVolume := false
	isClonePVC := false
	isImportImage := false
	sourcePVCName := ""
	sourcePVCNamespace := ""
	importSourceKind := ""
	importSourceURL := ""
	rootDataVolumeName := ""
	rootDataVolumeSize := ""
	rootDataVolumeStorageClass := strings.TrimSpace(spec.StorageClass)
	dataDiskGB := spec.DiskGB
	image := spec.Image
	naming := defaultVMReferenceNamingProfile
	switch {
	case strings.HasPrefix(spec.Image, "clone-pvc:"):
		hasRootDataVolume = true
		isClonePVC = true
		sourceRef := strings.TrimPrefix(spec.Image, "clone-pvc:")
		if idx := strings.LastIndex(sourceRef, "/"); idx >= 0 {
			sourcePVCNamespace = sourceRef[:idx]
			sourcePVCName = sourceRef[idx+1:]
		} else {
			sourcePVCName = sourceRef
		}
		if sourcePVCName == "" {
			return "", fmt.Errorf("render vm yaml: clone-pvc image source requires pvc name")
		}
		rootDataVolumeName = spec.Name + naming.RootDataVolumeSuffix
		if spec.DiskGB > 0 {
			rootDataVolumeSize = fmt.Sprintf("%dGi", spec.DiskGB)
		}
		dataDiskGB = 0
		image = "" // not used for clone-pvc
	case strings.HasPrefix(spec.Image, "import-image:"):
		hasRootDataVolume = true
		isImportImage = true
		var err error
		importSourceKind, importSourceURL, err = parseImportImageSource(strings.TrimPrefix(spec.Image, "import-image:"))
		if err != nil {
			return "", fmt.Errorf("render vm yaml: %w", err)
		}
		if spec.DiskGB <= 0 {
			return "", fmt.Errorf("render vm yaml: disk_gb must be > 0 for import-image boot source")
		}
		rootDataVolumeName = spec.Name + naming.RootDataVolumeSuffix
		rootDataVolumeSize = fmt.Sprintf("%dGi", spec.DiskGB)
		dataDiskGB = 0
		image = "" // not used for import-image
	case strings.HasPrefix(spec.Image, "pvc:"):
		return "", fmt.Errorf("render vm yaml: direct existing PVC boot is unsupported; use clone-pvc transport instead")
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
		Name:                       spec.Name,
		Namespace:                  namespace,
		Labels:                     spec.Labels,
		CPUCores:                   cpuCoresForTopology(cpuLimitCores),
		CPULimit:                   formatCPU(cpuLimitCores),
		CPURequest:                 formatCPU(cpuRequestCores),
		MemoryLimit:                memoryLimit,
		MemoryRequest:              memoryRequest,
		DataDiskGB:                 dataDiskGB,
		Image:                      image,
		CloudInit:                  spec.CloudInit,
		RootDiskName:               naming.RootDiskName,
		DataDiskName:               naming.DataDiskName,
		CloudInitDiskName:          naming.CloudInitDiskName,
		HasRootDataVolume:          hasRootDataVolume,
		IsClonePVC:                 isClonePVC,
		IsImportImage:              isImportImage,
		SourcePVCName:              sourcePVCName,
		SourcePVCNamespace:         sourcePVCNamespace,
		ImportSourceKind:           importSourceKind,
		ImportSourceURL:            importSourceURL,
		RootDataVolumeName:         rootDataVolumeName,
		RootDataVolumeSize:         rootDataVolumeSize,
		RootDataVolumeStorageClass: rootDataVolumeStorageClass,

		// DV storage mode (explicit fields — structural DV format change).
		DVAccessModes: spec.DVAccessModes,
		DVVolumeMode:  strings.TrimSpace(spec.DVVolumeMode),
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

func parseImportImageSource(raw string) (kind, url string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("import-image transport requires a non-empty source")
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "docker://"):
		return "registry", trimmed, nil
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return "http", trimmed, nil
	default:
		return "", "", fmt.Errorf("unsupported import-image source %q", trimmed)
	}
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
	// walk recursively expands nested JSON format keys (no dots in key)
	// into dot-notation leaf paths.
	var walk func(path string, value interface{})
	walk = func(path string, value interface{}) {
		nested, ok := value.(map[string]interface{})
		if !ok {
			flat[path] = value
			return
		}
		// Empty map = leaf value (e.g. rng: {}, guestAgentPing: {}).
		if len(nested) == 0 {
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
		// If the key already uses dot-notation (contains "."), it is a
		// fully-qualified path and its value should NOT be recursively
		// expanded. This preserves complex values like livenessProbe
		// objects, annotation maps, interface arrays, etc.
		if strings.Contains(path, ".") {
			flat[path] = value
		} else {
			// Nested JSON format: key like "spec" with value {"template": ...}
			walk(path, value)
		}
	}
	return flat
}

func cloneOverrideValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			cloned[key] = cloneOverrideValue(child)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, child := range typed {
			cloned[index] = cloneOverrideValue(child)
		}
		return cloned
	default:
		return typed
	}
}

func mergeNestedOverrideObject(target, override map[string]interface{}) map[string]interface{} {
	if target == nil {
		target = make(map[string]interface{}, len(override))
	}
	for key, rawValue := range override {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		nestedOverride, ok := rawValue.(map[string]interface{})
		if !ok {
			target[trimmed] = cloneOverrideValue(rawValue)
			continue
		}
		if len(nestedOverride) == 0 {
			target[trimmed] = map[string]interface{}{}
			continue
		}
		existingTarget, _ := target[trimmed].(map[string]interface{})
		target[trimmed] = mergeNestedOverrideObject(existingTarget, nestedOverride)
	}
	return target
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
		return scaleUintToInt64(uint64(v), 1000, "cpu")
	case uint64:
		return scaleUintToInt64(v, 1000, "cpu")
	case uint32:
		return scaleUintToInt64(uint64(v), 1000, "cpu")
	default:
		return 0, fmt.Errorf("unsupported cpu value type %T", value)
	}
}

func validateMemoryHalfStep(path string, value interface{}) error {
	memoryBytes, err := memoryValueToBytes(value)
	if err != nil {
		return fmt.Errorf("invalid spec_overrides value for %q: %w", path, err)
	}
	if memoryBytes <= 0 || memoryBytes%halfGiStepBytes != 0 {
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
		return scaleUintToInt64(uint64(v), 1024*1024, "memory")
	case uint64:
		return scaleUintToInt64(v, 1024*1024, "memory")
	case uint32:
		return scaleUintToInt64(uint64(v), 1024*1024, "memory")
	default:
		return 0, fmt.Errorf("unsupported memory value type %T (use Kubernetes quantity string, e.g. \"1536Mi\")", value)
	}
}

func scaleUintToInt64(v, factor uint64, field string) (int64, error) {
	const maxInt64 = ^uint64(0) >> 1
	if v > maxInt64/factor {
		return 0, fmt.Errorf("%s value %d overflows int64 after scaling", field, v)
	}
	return int64(v * factor), nil // #nosec G115 -- overflow checked by maxInt64/factor guard above.
}

// applySpecOverridesToYAML decodes YAML into an unstructured map, applies
// spec overrides, and re-encodes to YAML.
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
	baseObject, _ := cloneOverrideValue(obj.Object).(map[string]interface{})

	// Keep the original override shape here:
	// - dotted top-level keys continue to use SetNestedField
	// - nested JSON objects are deep-merged as literal maps
	//
	// This preserves legal additionalProperties keys such as
	// "kubevirt.io/ksm-enabled" inside nodeSelector/annotations.
	paths := make([]string, 0, len(overrides))
	for path := range overrides {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		trimmedPath := strings.TrimSpace(path)
		if trimmedPath == "" {
			continue
		}
		value := overrides[path]
		if strings.Contains(trimmedPath, ".") {
			segments := strings.Split(trimmedPath, ".")
			if err := unstructured.SetNestedField(obj.Object, cloneOverrideValue(value), segments...); err != nil {
				return "", fmt.Errorf("set spec override %q: %w", path, err)
			}
			continue
		}

		nestedValue, ok := value.(map[string]interface{})
		if !ok {
			obj.Object[trimmedPath] = cloneOverrideValue(value)
			continue
		}

		existingValue, found, err := unstructured.NestedMap(obj.Object, trimmedPath)
		if err != nil {
			return "", fmt.Errorf("get spec override %q: %w", path, err)
		}
		if !found {
			existingValue = nil
		}
		mergedValue := mergeNestedOverrideObject(existingValue, nestedValue)
		if err := unstructured.SetNestedField(obj.Object, mergedValue, trimmedPath); err != nil {
			return "", fmt.Errorf("set spec override %q: %w", path, err)
		}
	}

	if err := normalizeRenderedVMObject(obj.Object, baseObject, defaultVMReferenceNamingProfile); err != nil {
		return "", err
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

func normalizeRenderedVMObject(
	obj map[string]interface{},
	baseObject map[string]interface{},
	naming vmReferenceNamingProfile,
) error {
	if err := ensurePrimaryInterfaceBinding(obj, naming); err != nil {
		return err
	}
	if err := ensurePrimaryNetworkType(obj, naming); err != nil {
		return err
	}
	if err := ensureCloudInitVolumeForManagedDisk(obj, baseObject, naming); err != nil {
		return err
	}
	return nil
}

func ensurePrimaryInterfaceBinding(obj map[string]interface{}, naming vmReferenceNamingProfile) error {
	interfaces, found, err := unstructured.NestedSlice(
		obj,
		"spec", "template", "spec", "domain", "devices", "interfaces",
	)
	if err != nil || !found {
		return err
	}

	changed := false
	for index, raw := range interfaces {
		iface, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(iface["name"])) != naming.PrimaryNetworkName {
			continue
		}
		if hasAnyKey(iface, "binding", "bridge", "macvtap", "masquerade", "passt", "slirp", "sriov") {
			continue
		}
		iface["bridge"] = map[string]interface{}{}
		interfaces[index] = iface
		changed = true
	}

	if !changed {
		return nil
	}
	if err := unstructured.SetNestedSlice(
		obj,
		interfaces,
		"spec", "template", "spec", "domain", "devices", "interfaces",
	); err != nil {
		return fmt.Errorf("normalize interfaces: %w", err)
	}
	return nil
}

func ensurePrimaryNetworkType(obj map[string]interface{}, naming vmReferenceNamingProfile) error {
	networks, found, err := unstructured.NestedSlice(obj, "spec", "template", "spec", "networks")
	if err != nil {
		return err
	}
	if !found {
		networks = []interface{}{}
	}

	changed := false
	foundPrimary := false
	for index, raw := range networks {
		network, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(network["name"])) != naming.PrimaryNetworkName {
			continue
		}
		foundPrimary = true
		if hasAnyKey(network, "multus", "pod") {
			continue
		}
		network["pod"] = map[string]interface{}{}
		networks[index] = network
		changed = true
	}

	if !foundPrimary {
		return nil
	}
	if !changed {
		return nil
	}
	if err := unstructured.SetNestedSlice(obj, networks, "spec", "template", "spec", "networks"); err != nil {
		return fmt.Errorf("normalize networks: %w", err)
	}
	return nil
}

func ensureCloudInitVolumeForManagedDisk(
	obj map[string]interface{},
	baseObject map[string]interface{},
	naming vmReferenceNamingProfile,
) error {
	disks, found, err := unstructured.NestedSlice(obj, "spec", "template", "spec", "domain", "devices", "disks")
	if err != nil || !found {
		return err
	}

	needsCloudInitVolume := false
	for _, raw := range disks {
		disk, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(disk["name"])) == naming.CloudInitDiskName {
			needsCloudInitVolume = true
			break
		}
	}
	if !needsCloudInitVolume {
		return nil
	}

	volumes, found, err := unstructured.NestedSlice(obj, "spec", "template", "spec", "volumes")
	if err != nil {
		return err
	}
	if !found {
		volumes = []interface{}{}
	}
	for _, raw := range volumes {
		volume, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(volume["name"])) == naming.CloudInitDiskName {
			return nil
		}
	}

	if baseVolume, ok := findNamedVolume(baseObject, naming.CloudInitDiskName); ok {
		volumes = append(volumes, cloneOverrideValue(baseVolume))
		if err := unstructured.SetNestedSlice(obj, volumes, "spec", "template", "spec", "volumes"); err != nil {
			return fmt.Errorf("normalize cloud-init volume: %w", err)
		}
		return nil
	}

	filteredDisks := removeNamedItems(disks, naming.CloudInitDiskName)
	if len(filteredDisks) != len(disks) {
		if err := unstructured.SetNestedSlice(
			obj,
			filteredDisks,
			"spec", "template", "spec", "domain", "devices", "disks",
		); err != nil {
			return fmt.Errorf("normalize cloud-init disk: %w", err)
		}
	}
	return nil
}

func hasAnyKey(values map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func findNamedVolume(obj map[string]interface{}, name string) (map[string]interface{}, bool) {
	if obj == nil {
		return nil, false
	}
	volumes, found, err := unstructured.NestedSlice(obj, "spec", "template", "spec", "volumes")
	if err != nil || !found {
		return nil, false
	}
	for _, raw := range volumes {
		volume, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(volume["name"])) == name {
			return volume, true
		}
	}
	return nil, false
}

func removeNamedItems(items []interface{}, name string) []interface{} {
	filtered := make([]interface{}, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		if strings.TrimSpace(fmt.Sprint(item["name"])) == name {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// VMRenderInput contains the fields needed to render a VM YAML template.
// This is a projection of domain.VMSpec with all fields needed for rendering.
//
// Resource granularity: All CPU/Memory values must be in 0.5-step increments.
// Non-standard values (0.7, 1.2, etc.) are rejected at render time.
type VMRenderInput struct {
	Name     string
	CPUCores float64 // CPU limit in cores (0.5 step: 0.5, 1.0, 1.5, ...)
	MemoryGi float64 // Memory limit in Gi (0.5 step: 0.5, 1.0, 1.5, ...)
	DiskGB   int     // Desired root disk size for CDI-backed boot sources.
	// Image accepts one of:
	//   - container disk image reference: "quay.io/containerdisks/ubuntu:22.04"
	//   - CDI registry/http import: "import-image:<docker://...|https://...>"
	//   - CDI clone source PVC: "clone-pvc:<claim>" or "clone-pvc:<namespace>/<claim>"
	//
	// Direct existing PVC transport ("pvc:<claim>") is intentionally unsupported.
	Image        string
	StorageClass string
	CloudInit    string
	Labels       map[string]string
	// CPURequest is for overcommit: CPU request in cores (must be <= CPUCores).
	CPURequest float64
	// MemoryRequestGi is for overcommit: Memory request in Gi (must be <= MemoryGi).
	MemoryRequestGi float64
	// SpecOverrides carries advanced KubeVirt spec path/value overrides (ADR-0018 Hybrid Model).
	// Keys are dot-notation paths starting with "spec." prefix.
	// Applied as deep-merge patches after template rendering.
	SpecOverrides map[string]interface{}

	// DVAccessModes sets the DataVolume PVC access mode(s), e.g. ["ReadWriteMany"].
	// When set, the renderer uses the CDI 'pvc' format instead of 'storage' format.
	// This is an explicit field because it changes the DV YAML structure.
	DVAccessModes []string
	// DVVolumeMode sets the DataVolume PVC volume mode: "Block" or "Filesystem".
	DVVolumeMode string
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
