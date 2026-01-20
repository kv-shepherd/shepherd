# ADR-0014: KubeVirt Capability Detection and Template Compatibility Strategy

> **Status**: Accepted  
> **Date**: 2026-01-18

---

## Context

### Problem

The governance platform needs to manage multiple KubeVirt clusters running different versions with different Feature Gates enabled. When users select a VM template and deploy to a target cluster, there are compatibility risks:

1. **Version differences**: Fields used in template may not exist in target cluster's KubeVirt version
2. **Feature Gate differences**: Features required by template (GPU passthrough, Snapshot) may not be enabled
3. **Hardware differences**: Target cluster may lack required hardware resources (GPU, SRIOV NICs)

### Design Goals

1. **Decouple templates from clusters**: Templates not bound to specific clusters, reusable across clusters
2. **Pre-filter incompatible clusters**: After user selects template, only show compatible clusters
3. **Minimal implementation complexity**: Single developer, control workload
4. **Reliability guarantee**: Dry run as final fallback

---

## Decision

### Adopt: Runtime Capability Detection + Dry Run Fallback

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Capability Detection Architecture                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Layer 1: Runtime Detection (executed during health check)                  │
│  ├── ServerVersion().Get() → Get KubeVirt version                           │
│  ├── KubeVirt CR → Read spec.configuration...featureGates                   │
│  └── GA Features Static Table → Supplement default-enabled GA features      │
│                                                                              │
│  Layer 2: Template Metadata                                                  │
│  ├── required_features: List of features template depends on                │
│  └── Specified by admin/creator when creating template                      │
│                                                                              │
│  Layer 3: Runtime Validation                                                 │
│  ├── After selecting template, filter to show compatible clusters only      │
│  └── Dry run fallback before submit (catches remaining issues)              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation

### 1. Cluster Capability Detection

```go
// internal/provider/kubevirt_capability_detector.go

// ClusterCapabilities cluster capability information
type ClusterCapabilities struct {
    Version         string   // KubeVirt version, e.g., "v1.7.0"
    EnabledFeatures []string // All enabled features (explicit + GA)
    DetectedAt      time.Time
}

// Detect inspects cluster's KubeVirt capabilities
// Minimal implementation: 2 GET requests
func (d *CapabilityDetector) Detect(ctx context.Context, restConfig *rest.Config) (*ClusterCapabilities, error) {
    client, _ := kubecli.GetKubevirtClientFromRESTConfig(restConfig)
    
    // 1. Get version
    versionInfo, _ := client.ServerVersion().Get()
    
    // 2. Get KubeVirt CR
    kv, _ := client.KubeVirt("kubevirt").Get(ctx, "kubevirt", metav1.GetOptions{})
    
    // 3. Extract enabled feature gates (explicitly configured)
    var enabledFeatures []string
    if kv.Spec.Configuration.DeveloperConfiguration != nil {
        enabledFeatures = kv.Spec.Configuration.DeveloperConfiguration.FeatureGates
    }
    
    // 4. Add GA features (default enabled)
    gaFeatures := getGAFeaturesForVersion(versionInfo.GitVersion)
    allFeatures := mergeUnique(enabledFeatures, gaFeatures)
    
    return &ClusterCapabilities{
        Version:         versionInfo.GitVersion,
        EnabledFeatures: allFeatures,
        DetectedAt:      time.Now(),
    }, nil
}
```

### 2. GA Features Static Table

```go
// gaFeaturesByVersion GA features static table
// Only table requiring maintenance, updated once per major version (~every 6 months)
// Source: https://kubevirt.io/user-guide/cluster_admin/activating_feature_gates/
var gaFeaturesByVersion = map[string][]string{
    "v1.0": {"LiveMigration"},
    "v1.4": {"LiveMigration", "NetworkHotplug", "CommonInstancetypesDeployment"},
    "v1.5": {"LiveMigration", "NetworkHotplug", "CommonInstancetypesDeployment", "NUMA"},
    "v1.6": {"LiveMigration", "NetworkHotplug", "CommonInstancetypesDeployment", "NUMA", "GPUAssignment"},
    "v1.7": {"LiveMigration", "NetworkHotplug", "CommonInstancetypesDeployment", "NUMA", "GPUAssignment", "NodeRestriction"},
}
```

### 3. Template Schema Extension

```go
// ent/schema/template.go additional fields

field.JSON("required_features", []string{}).
    Optional().
    Comment("Required feature gates, e.g., [\"GPU\", \"Snapshot\"]"),

field.JSON("required_hardware", []string{}).
    Optional().
    Comment("Required hardware, e.g., [\"nvidia.com/gpu\", \"intel.com/sriov\"]"),
```

### 4. Cluster Capability Storage

```go
// ent/schema/cluster.go additional fields

field.JSON("capabilities", ClusterCapabilities{}).
    Optional().
    Comment("Auto-detected cluster capabilities"),

field.Time("capabilities_updated_at").
    Optional().
    Comment("Last capability detection time"),
```

---

## Compatibility Filtering

```go
// internal/service/cluster_service.go

// ListCompatibleClusters filters clusters compatible with template requirements
func (s *ClusterService) ListCompatibleClusters(
    ctx context.Context,
    templateID int,
) ([]*ent.Cluster, error) {
    // 1. Get template
    tmpl, _ := s.templateRepo.Get(ctx, templateID)
    
    // 2. Query all available clusters
    clusters, _ := s.clusterRepo.ListHealthy(ctx)
    
    // 3. Filter compatible clusters
    var compatible []*ent.Cluster
    for _, cluster := range clusters {
        if isCompatible(cluster.Capabilities, tmpl.RequiredFeatures, tmpl.RequiredHardware) {
            compatible = append(compatible, cluster)
        }
    }
    
    return compatible, nil
}

func isCompatible(caps ClusterCapabilities, requiredFeatures, requiredHardware []string) bool {
    // All required features must be present in cluster's enabled features
    for _, feature := range requiredFeatures {
        if !contains(caps.EnabledFeatures, feature) {
            return false
        }
    }
    // Hardware checks require separate node resource detection (simplified for v1)
    return true
}
```

---

## Dry Run Fallback

```go
// Before actual submission, execute DryRun
func (s *VMService) CreateVM(ctx context.Context, ...) error {
    // 1. Render template
    yamlData, _ := s.renderer.Render(template.Content, params)
    
    // 2. DryRun validation (final fallback)
    if err := s.ssaApplier.DryRunApply(ctx, yamlData); err != nil {
        return fmt.Errorf("DryRun failed: %w - cluster may lack required capabilities", err)
    }
    
    // 3. Actual creation...
}
```

---

## User Workflow

```
1. User selects template (contains required_features metadata)
        ↓
2. Frontend calls API: GET /api/v1/clusters?compatible_with_template=xxx
        ↓
3. Backend filters clusters based on:
   - Cluster capabilities.enabled_features ⊇ Template required_features
   - (Future) Hardware requirements
        ↓
4. User selects compatible cluster
        ↓
5. Submit creation request
        ↓
6. Dry run final validation (catches edge cases)
        ↓
7. Actual execution
```

---

## Maintenance Requirements

### GA Features Table Update

| When | Action |
|------|--------|
| KubeVirt minor release | Check Release Notes for Alpha→GA promotions |
| Add new table entry | Add version key with updated feature list |
| Estimated time | 15-30 minutes per release |

---

## Consequences

### Positive

- ✅ **Templates decoupled from clusters**: Templates reusable across environments
- ✅ **User-friendly**: Only compatible clusters shown
- ✅ **Low complexity**: Runtime detection, no additional infrastructure
- ✅ **Reliable**: Dry run catches missed compatibility issues

### Negative

- 🟡 **Maintenance required**: GA features table needs periodic updates
- 🟡 **Timing gap**: Capability may change between detection and create
- 🟡 **Hardware detection limited**: Full hardware detection deferred

### Mitigation

- Dry run catches timing gaps
- Set up calendar reminder for GA table maintenance
- Hardware detection can be enhanced incrementally

---

## References

- [KubeVirt Feature Gates Documentation](https://kubevirt.io/user-guide/cluster_admin/activating_feature_gates/)
- [ADR-0011: SSA Apply Strategy](./ADR-0011-ssa-apply-strategy.md)
