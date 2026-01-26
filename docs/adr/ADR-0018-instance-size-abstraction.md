# ADR-0018: Instance Size Abstraction Layer (Schema-Driven Design)

> **Status**: Proposed  
> **Date**: 2026-01-22  
> **Review Period**: Until **2026-01-28** (48-hour public comment period)  
> **Discussion Issue**: [Issue #17](https://github.com/kv-shepherd/shepherd/issues/17)  
> **Supersedes**: Previous ADR-0018 drafts  
> **Amends**: ADR-0015 §5 (Template Layered Design)  
> **Relates To**: ADR-0014 (Capability Detection), ADR-0015 (Governance Model V2), ADR-0017 (VM Request Flow)

---

## Amendment Notice

> The following decisions from ADR-0015 are **amended** by this ADR:

| ADR-0015 Section | Original Decision | Amendment in ADR-0018 |
|------------------|-------------------|----------------------|
| §5. Template Layered Design | Template contains `required_features`, `required_hardware` capability requirements | **MOVED** to InstanceSize. See [§4. Backend Storage](#4-backend-storage-dumb). Template now only contains: OS image source, cloud-init config, field visibility control. |
| §5. Template Layered Design | Template defines `quick_fields` and `advanced_fields` for field visibility | **CLARIFIED**: InstanceSize now defines hardware capabilities (GPU/SR-IOV/Hugepages). Template retains field visibility control for UI rendering only. |

> **Note**: ADR-0015 remains **Accepted**. This amendment is a refinement, not a replacement. Cross-reference this ADR when implementing Template and InstanceSize features.

---

## Design Changes Summary

> This section summarizes major design changes from earlier drafts.

### Deprecated Decisions (Do NOT Implement)

The following decisions from earlier ADRs and drafts are **DEPRECATED** and should NOT be implemented:

> **ADR Immutability Principle**: Per ADR best practices, accepted ADRs are immutable historical records. Deprecated decisions from accepted ADRs are superseded by this ADR, but the original ADRs remain unchanged. Upon acceptance of ADR-0018, an "Amendments by Subsequent ADRs" section will be appended to affected ADRs to provide cross-reference.

| Deprecated Feature | Source ADR/Document | Previous Design | Reason for Deprecation | Current Design |
|--------------------|---------------------|-----------------|------------------------|----------------|
| Template capability requirements | **[ADR-0015 §5](./ADR-0015-governance-model-v2.md#5-template-layered-design-quick--advanced)**, **[ADR-0014](./ADR-0014-capability-detection.md)** | Template stored `required_features`, `required_hardware` | Capability requirements are hardware-related, should be with InstanceSize | InstanceSize stores all hardware requirements |
| Template YAML editor | *(Earlier drafts, not in accepted ADR)* | Admin edits raw YAML for template content | Complex UX, error-prone | Form-based: image source selector + cloud-init YAML only |
| Go Template variables in cloud-init | *(Earlier drafts, not in accepted ADR)* | `{{ .Username }}`, `{{ .SSHPublicKey }}` injected at render time | Unclear variable source, over-engineering | Simple one-time password, user manages post-creation |
| Platform manages SSH keys | *(Earlier drafts, not in accepted ADR)* | Platform stores and injects user SSH keys | Out of scope, security complexity | Platform provides initial password only; bastion/SSH key management is user/admin responsibility |

### Responsibility Boundary Clarification

| Responsibility | Platform Scope | NOT Platform Scope |
|----------------|----------------|-------------------|
| **VM Initialization** | Provide one-time password for first login | SSH key management, bastion integration |
| **Namespace** | Optional creation helper | K8s RBAC, ResourceQuota management |
| **Hardware Capabilities** | Configured in InstanceSize | ~~Configured in Template~~ |
| **Cluster Matching** | Environment type matching (test→test, prod→prod) | Cross-environment scheduling |

### Configuration Storage Strategy (Added 2026-01-26)

> **Decision**: All runtime configuration is stored in PostgreSQL. Only infrastructure-level settings use config file or environment variables.

**Rationale**:
- All configuration changes have audit logs
- All configuration is manageable via Web UI
- No YAML files to maintain or synchronize for runtime config
- Flexible deployment: config.yaml for local development, env vars for containers

**Configuration Layers**:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Configuration Storage Strategy                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Layer 1: Infrastructure Configuration (config.yaml OR env vars)        │
│  ─────────────────────────────────────────────────────────────────       │
│                                                                          │
│  📁 Option A: config.yaml (for local development / traditional deploy)  │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  # config.yaml                                                      │ │
│  │  database:                                                          │ │
│  │    url: "postgresql://user:pass@localhost:5432/shepherd"            │ │
│  │                                                                      │ │
│  │  server:                                                             │ │
│  │    port: 8080                                                        │ │
│  │    log_level: "info"                # Optional, default: info        │ │
│  │                                                                      │ │
│  │  worker:                                                             │ │
│  │    max_workers: 10                  # Optional, default: 10          │ │
│  │                                                                      │ │
│  │  security:                                                           │ │
│  │    encryption_key: "32-byte-hex"    # Optional, for encrypting secrets│ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  🐳 Option B: Environment Variables (for containerized deployment)      │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  DATABASE_URL=postgresql://user:pass@host:5432/shepherd  # Required  │ │
│  │  SERVER_PORT=8080                   # Optional, default: 8080        │ │
│  │  LOG_LEVEL=info                     # Optional, default: info        │ │
│  │  RIVER_MAX_WORKERS=10               # Optional, default: 10          │ │
│  │  ENCRYPTION_KEY=<32-byte-hex>       # Optional, encrypt secrets      │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ⚡ Priority: Environment Variables > config.yaml > defaults            │
│  💡 Env vars always override config.yaml (12-factor app principle)      │
│                                                                          │
│  Layer 2: PostgreSQL (All Runtime Configuration)                        │
│  ──────────────────────────────────────────────────                      │
│  • users                           # Local users (JWT auth)              │
│  • auth_providers                  # OIDC/LDAP config (Web UI)           │
│  • idp_group_mappings              # IdP group → role mapping            │
│  • external_approval_systems       # External approval integration       │
│  • roles                           # Built-in + custom roles             │
│  • role_bindings                   # Permission bindings                 │
│  • resource_role_bindings          # Resource-level permissions          │
│  • clusters                        # Cluster configuration               │
│  • instance_sizes                  # InstanceSize configuration          │
│  • templates                       # Template configuration              │
│  • systems/services/vms            # Business data                       │
│  • audit_logs                      # All change records                  │
│                                                                          │
│  Layer 3: Code-Embedded (Version-Controlled)                            │
│  ──────────────────────────────────────────────                          │
│  • JSON Schema                     # KubeVirt field definitions          │
│  • Mask Configuration              # Exposed field paths                 │
│  • Built-in Role Definitions       # Seed data (see below)               │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**First Deployment Flow**:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    First Deployment Bootstrap                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. Deploy with DATABASE_URL only                                        │
│                                                                          │
│  2. Application auto-initializes:                                        │
│     • Run migrations                                                     │
│     • Seed built-in roles (IF NOT EXISTS)                               │
│     • Seed default admin: admin/admin (force_password_change=true)      │
│                                                                          │
│  3. First login:                                                         │
│     ┌───────────────────────────────────────────────────────────────┐   │
│     │  ⚠️ Default credentials detected                              │   │
│     │                                                                │   │
│     │  Username: admin                                               │   │
│     │  Password: admin                                               │   │
│     │                                                                │   │
│     │  [Login]                                                       │   │
│     └───────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  4. Force password change:                                               │
│     ┌───────────────────────────────────────────────────────────────┐   │
│     │  🔐 Please set a new password                                  │   │
│     │                                                                │   │
│     │  New Password: ********                                        │   │
│     │  Confirm: ********                                             │   │
│     │                                                                │   │
│     │  [Confirm]                                                     │   │
│     └───────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  5. Enter admin console:                                                 │
│     • Configure OIDC/LDAP (optional)                                    │
│     • Configure External Approval Systems (optional)                    │
│     • Configure Clusters/InstanceSize/Templates                         │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Sensitive Data Encryption**:

| Data | Encryption Method | Notes |
|------|-------------------|-------|
| `users.password_hash` | bcrypt | Never store plaintext |
| `auth_providers.oidc_client_secret` | AES-256-GCM | App-level encryption |
| `auth_providers.ldap_bind_password` | AES-256-GCM | App-level encryption |
| `external_approval_systems.webhook_secret` | AES-256-GCM | App-level encryption |

> **Encryption Key Management**: Use `ENCRYPTION_KEY` environment variable (32-byte hex string) for app-level encryption. For production, recommend using external KMS (Vault, AWS KMS).

**Built-in Role Seeding Logic**:

```go
// On application startup
func SeedBuiltinRoles(ctx context.Context, db *sql.DB) error {
    builtinRoles := []Role{
        {ID: "platform-admin", Name: "PlatformAdmin", Permissions: []string{"*:*"}, IsBuiltin: true},
        {ID: "system-admin", Name: "SystemAdmin", Permissions: []string{"system:*", "service:*", "vm:*"}, IsBuiltin: true},
        {ID: "approver", Name: "Approver", Permissions: []string{"approval:*", "vm:read"}, IsBuiltin: true},
        {ID: "operator", Name: "Operator", Permissions: []string{"vm:operate"}, IsBuiltin: true},
        {ID: "viewer", Name: "Viewer", Permissions: []string{"*:read"}, IsBuiltin: true},
    }
    
    for _, role := range builtinRoles {
        _, err := db.ExecContext(ctx, `
            INSERT INTO roles (id, name, permissions, is_builtin, created_at)
            VALUES ($1, $2, $3, $4, NOW())
            ON CONFLICT (id) DO NOTHING  -- Skip if exists, don't overwrite
        `, role.ID, role.Name, role.Permissions, role.IsBuiltin)
        if err != nil {
            return err
        }
    }
    return nil
}
```

---

## Context

### Problem Statement

KubeVirt VirtualMachine API has hundreds of fields. Users need a simple way to configure VMs without understanding the full YAML structure. The platform needs to:

1. Expose relevant fields to users through friendly UI
2. Store user configurations without understanding their semantics
3. Render configurations into valid VirtualMachine YAML

### Core Design Principles

> **Critical**: These principles MUST NOT be violated.

| Principle | Description |
|-----------|-------------|
| **Schema as Source of Truth** | KubeVirt official JSON Schema defines all field types, constraints, and enum options. We do NOT duplicate this in our code. |
| **Mask Selects Paths** | Mask only specifies which JSON Schema paths to expose. It does NOT define field options or values. |
| **Dumb Backend** | Backend stores `map[string]interface{}` and does NOT interpret field semantics (e.g., what is "GPU"). |
| **Schema-Driven UI** | Frontend reads JSON Schema + Mask to render appropriate UI components based on field types. |

---

## Decision

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    KubeVirt Official JSON Schema                             │
│                    (Source of Truth for Field Types)                         │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ Mask references paths
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Mask Configuration                                        │
│                    (Selects which paths to expose)                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ Frontend renders based on Schema + Mask
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Admin/User UI                                             │
│                    (Users fill in values based on Schema types)              │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ Submit as JSON
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Backend Storage (PostgreSQL)                              │
│                    spec_overrides: map[string]interface{}                    │
│                    (Backend does NOT interpret contents)                     │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ Merge with Template
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Final VirtualMachine YAML                                 │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Component Details

#### 1. KubeVirt JSON Schema (External, Official)

**Source**: KubeVirt CRD OpenAPI Schema or https://kubevirt.io/api-reference/

The schema defines all field types, constraints, and enum options:

```json
{
  "spec.template.spec.domain.cpu.cores": {
    "type": "integer",
    "minimum": 1
  },
  "spec.template.spec.domain.memory.hugepages.pageSize": {
    "type": "string",
    "enum": ["2Mi", "1Gi"]  // Options come from official schema
  },
  "spec.template.spec.domain.devices.gpus": {
    "type": "array",
    "items": {
      "type": "object",
      "properties": {
        "name": { "type": "string" },
        "deviceName": { "type": "string" }  // User types freely
      }
    }
  }
}
```

#### 2. Mask Configuration (Developer-Defined)

Mask only specifies **which paths to expose** and **how to display them**. It does NOT define field options.

```yaml
# config/mask.yaml
version: "1.0"

quick_fields:
  - path: "spec.template.spec.domain.cpu.cores"
    display_name: "CPU Cores"
    description: "Number of CPU cores"
    
  - path: "spec.template.spec.domain.resources.requests.memory"
    display_name: "Memory"
    description: "Memory size (e.g., 8Gi)"

advanced_fields:
  - path: "spec.template.spec.domain.devices.gpus"
    display_name: "GPU Devices"
    description: "GPU passthrough configuration"
    
  - path: "spec.template.spec.domain.memory.hugepages.pageSize"
    display_name: "Hugepages Size"
    description: "Hugepages page size"
    
  - path: "spec.template.spec.domain.cpu.dedicatedCpuPlacement"
    display_name: "Dedicated CPU"
    description: "Enable dedicated CPU placement"
```

#### 3. Frontend Rendering (Schema-Driven)

Frontend reads JSON Schema + Mask and auto-renders UI based on field types:

| Schema Type | UI Component |
|-------------|--------------|
| `integer` | Number input |
| `string` | Text input |
| `boolean` | Checkbox |
| `string` with `enum` | Dropdown (options from Schema) |
| `array` | Dynamic add/remove table |
| `object` | Nested form group |

Example UI rendering:

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Create Instance Size                                                     │
│                                                                           │
│  Name:        [gpu-workstation    ]                                       │
│  Display:     [GPU Workstation (8 vCPU, 32GB)]                           │
│                                                                           │
│  ── Resource Settings ──                                                  │
│  CPU Cores:   [8        ]         (integer → number input)                │
│  [✓] Enable CPU Overcommit   👈 When checked, show request/limit          │
│      ┌──────────────────────────────────────────────────────────────┐    │
│      │  CPU Request: [4    ]   CPU Limit: [8    ]   (2x overcommit) │    │
│      └──────────────────────────────────────────────────────────────┘    │
│                                                                           │
│  Memory:      [32Gi     ]         (string → text input)                   │
│  [✓] Enable Memory Overcommit                                             │
│      ┌──────────────────────────────────────────────────────────────┐    │
│      │  Mem Request: [16Gi ]   Mem Limit: [32Gi ]   (2x overcommit) │    │
│      └──────────────────────────────────────────────────────────────┘    │
│                                                                           │
│  ── Advanced Settings ──                                                  │
│  Hugepages:   [2Mi ▼]             (enum → dropdown from Schema)           │
│               [2Mi ]                                                      │
│               [1Gi ]              ← Options from KubeVirt Schema          │
│                                                                           │
│  Dedicated CPU: [✓]               (boolean → checkbox)                    │
│                 ⚠️ Warning: Incompatible with CPU overcommit!             │
│                                                                           │
│  GPU Devices:                     (array → dynamic table)                 │
│  ┌──────────────────────────────────────────────────────────────────┐    │
│  │  Name       Device Name                                           │    │
│  │  [gpu1   ]  [nvidia.com/GA102GL_A10         ] ← User types freely │    │
│  │                                                                    │    │
│  │  [+ Add GPU]                                                       │    │
│  └──────────────────────────────────────────────────────────────────┘    │
│                                                                           │
│  [Save]                                                                   │
└──────────────────────────────────────────────────────────────────────────┘
```

#### 4. Backend Storage (Dumb)

Backend stores user input as generic JSON. It does NOT parse or interpret the contents.

```go
// ent/schema/instance_size.go
func (InstanceSize) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("name").NotEmpty().Unique(),       // "gpu-workstation"
        field.String("display_name").NotEmpty(),        // "GPU Workstation"
        
        // Default disk size (Admin preset, User can adjust)
        field.Int("default_disk_gb").Default(100).
            Comment("Admin preset default disk size. User can adjust via slider."),
        field.Int("min_disk_gb").Default(50).
            Comment("Minimum disk size allowed."),
        field.Int("max_disk_gb").Default(500).
            Comment("Maximum disk size allowed."),
        
        // Overcommit configuration (optional)
        field.JSON("cpu_overcommit", &OvercommitConfig{}).Optional().
            Comment("CPU overcommit config: {enabled, request, limit}"),
        field.JSON("mem_overcommit", &OvercommitConfig{}).Optional().
            Comment("Memory overcommit config: {enabled, request, limit}"),
        
        // Generic JSON storage - backend does NOT interpret contents
        field.JSON("spec_overrides", map[string]interface{}{}).
            Comment("JSON Path → Value mapping. Backend does NOT interpret."),
        
        field.Bool("enabled").Default(true),
        field.Time("created_at").Default(time.Now),
    }
}

// OvercommitConfig defines request/limit for resource overcommit
type OvercommitConfig struct {
    Enabled bool   `json:"enabled"`    // Whether overcommit is enabled
    Request string `json:"request"`    // e.g., "4" for CPU, "16Gi" for memory
    Limit   string `json:"limit"`      // e.g., "8" for CPU, "32Gi" for memory
}
```

Example stored data:

```json
{
  "id": "is-001",
  "name": "gpu-workstation",
  "display_name": "GPU Workstation (8 vCPU, 32GB)",
  "default_disk_gb": 100,
  "min_disk_gb": 50,
  "max_disk_gb": 500,
  "cpu_overcommit": {
    "enabled": true,
    "request": "4",
    "limit": "8"
  },
  "mem_overcommit": {
    "enabled": true,
    "request": "16Gi",
    "limit": "32Gi"
  },
  "spec_overrides": {
    "spec.template.spec.domain.cpu.cores": 8,
    "spec.template.spec.domain.resources.requests.memory": "32Gi",
    "spec.template.spec.domain.memory.hugepages.pageSize": "2Mi",
    "spec.template.spec.domain.cpu.dedicatedCpuPlacement": true,
    "spec.template.spec.domain.devices.gpus": [
      {"name": "gpu1", "deviceName": "nvidia.com/GA102GL_A10"}
    ]
  }
}
```

#### 5. Overcommit Configuration (Request/Limit Model)

> **Key Insight**: Admin can optionally enable overcommit per InstanceSize with explicit request/limit values. Users see the "limit" value as the advertised spec.

**Phase 1: Admin Creates InstanceSize with Overcommit Option**

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│  Create InstanceSize                                                              │
│                                                                                   │
│  Name:         [medium              ]                                             │
│  Display Name: [Medium (4 vCPU, 8GB) ]                                            │
│                                                                                   │
│  ── CPU Configuration ──────────────────────────────────────────────────────────  │
│  CPU Cores:    [4        ]                                                        │
│  [✓] Enable Overcommit     👈 When checked, show request/limit fields             │
│                                                                                   │
│      ┌────────────────────────────────────────────────────────────────────────┐   │
│      │  CPU Request: [2    ] cores   CPU Limit: [4    ] cores                 │   │
│      │               ───────────────────────────────────────                  │   │
│      │               Example: request=2, limit=4 means 2x overcommit          │   │
│      └────────────────────────────────────────────────────────────────────────┘   │
│                                                                                   │
│  ── Memory Configuration ───────────────────────────────────────────────────────  │
│  Memory:       [8Gi      ]                                                        │
│  [✓] Enable Overcommit                                                            │
│                                                                                   │
│      ┌────────────────────────────────────────────────────────────────────────┐   │
│      │  Mem Request: [4Gi  ]         Mem Limit: [8Gi  ]                       │   │
│      │               ─────────────────────────────────                        │   │
│      │               Example: request=4Gi, limit=8Gi means 2x overcommit      │   │
│      └────────────────────────────────────────────────────────────────────────┘   │
│                                                                                   │
│  [Save]                                                                           │
└──────────────────────────────────────────────────────────────────────────────────┘

  👆 If overcommit NOT checked → CPU Request = CPU Limit = 4 (no overcommit)
     If overcommit IS checked  → Admin explicitly sets request < limit
```

**Updated InstanceSize Schema**:

```go
type InstanceSize struct {
    // ... existing fields
    
    // Overcommit configuration (optional)
    CPUOvercommit *OvercommitConfig `json:"cpu_overcommit,omitempty"`
    MemOvercommit *OvercommitConfig `json:"mem_overcommit,omitempty"`
}

type OvercommitConfig struct {
    Enabled  bool   `json:"enabled"`           // Whether overcommit is enabled
    Request  string `json:"request"`           // e.g., "2" for CPU, "4Gi" for memory
    Limit    string `json:"limit"`             // e.g., "4" for CPU, "8Gi" for memory
}
```

**Stored Example**:

```json
{
  "name": "medium",
  "display_name": "Medium (4 vCPU, 8GB)",
  "cpu_overcommit": {
    "enabled": true,
    "request": "2",
    "limit": "4"
  },
  "mem_overcommit": {
    "enabled": true,
    "request": "4Gi",
    "limit": "8Gi"
  },
  "spec_overrides": { ... }
}
```

**Phase 2: Admin Approval with Overcommit Adjustment**

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│  Approve VM Request                                                               │
│                                                                                   │
│  Request Details:                                                                 │
│  ───────────────────────────────────────────────────────────────────────────────  │
│  Requester:    zhang.san                                                          │
│  Namespace:    prod-shop              👈 prod environment                         │
│  Service:      shop/redis                                                         │
│  InstanceSize: medium (4 vCPU, 8GB)                                               │
│                                                                                   │
│  ── Resource Allocation ────────────────────────────────────────────────────────  │
│                                                                                   │
│  [✓] Enable Override    👈 Admin can override default request/limit values        │
│                                                                                   │
│  ┌────────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                            │   │
│  │  CPU:    Request [2    ] cores   Limit [4    ] cores                       │   │
│  │  Memory: Request [4Gi  ]         Limit [8Gi  ]                             │   │
│  │                                                                            │   │
│  │  ⚠️ WARNING: Overcommit enabled for PRODUCTION environment!               │   │
│  │     This may impact VM performance under high load.                        │   │
│  │                                                                            │   │
│  └────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                   │
│  Cluster:      [cluster-a ▼]                                                      │
│                                                                                   │
│  [Approve]  [Reject]                                                              │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**Approval Logic**:

```go
func ApproveVMRequest(request *VMRequest, approvalOverride *OverrideConfig) (*Approval, []Warning) {
    var warnings []Warning
    
    instanceSize := GetInstanceSize(request.InstanceSizeID)
    namespace := GetNamespace(request.NamespaceID)
    
    // Default: use InstanceSize's overcommit config
    cpuRequest := instanceSize.CPUOvercommit.Request
    cpuLimit := instanceSize.CPUOvercommit.Limit
    memRequest := instanceSize.MemOvercommit.Request
    memLimit := instanceSize.MemOvercommit.Limit
    
    // Admin override (if provided)
    if approvalOverride != nil && approvalOverride.Enabled {
        cpuRequest = approvalOverride.CPURequest
        cpuLimit = approvalOverride.CPULimit
        memRequest = approvalOverride.MemRequest
        memLimit = approvalOverride.MemLimit
    }
    
    // Warning for prod environment with overcommit (request < limit)
    if namespace.Environment == "prod" {
        if cpuRequest != cpuLimit || memRequest != memLimit {
            warnings = append(warnings, Warning{
                Level:   "WARNING",  // Yellow warning, NOT blocking
                Message: "Overcommit enabled for PRODUCTION environment. This may impact VM performance.",
            })
        }
    }
    
    // Warning for conflicting configurations (overcommit + dedicated CPU)
    hasOvercommit := cpuRequest != cpuLimit || memRequest != memLimit
    hasDedicatedCPU := instanceSize.SpecOverrides["spec.template.spec.domain.cpu.dedicatedCpuPlacement"] == true
    
    if hasOvercommit && hasDedicatedCPU {
        warnings = append(warnings, Warning{
            Level:   "WARNING",  // Yellow warning, NOT blocking
            Message: "CONFLICT: Dedicated CPU Placement is incompatible with CPU overcommit. VM may fail to start.",
        })
    }
    
    // Approval proceeds regardless of warnings (user takes responsibility)
    return &Approval{
        CPURequest: cpuRequest,
        CPULimit:   cpuLimit,
        MemRequest: memRequest,
        MemLimit:   memLimit,
    }, warnings
}
```

**User Experience Summary**:

| Actor | What they see | What they control |
|-------|--------------|-------------------|
| **User** | "Medium (4 vCPU, 8GB)" | Nothing about overcommit |
| **Admin (Create)** | Request/Limit fields when overcommit enabled | Default overcommit ratio per InstanceSize |
| **Admin (Approve)** | Override toggle + Warnings | Final request/limit values per VM |

**Warning Types** (Yellow warning, NOT blocking):

| Condition | Warning Message |
|-----------|-----------------|
| Overcommit in prod environment | "Overcommit enabled for PRODUCTION environment. This may impact VM performance." |
| Overcommit + Dedicated CPU | "CONFLICT: Dedicated CPU Placement is incompatible with CPU overcommit. VM may fail to start." |

> **Note**: Warnings are advisory only. Admin takes responsibility for the final configuration.

#### 6. VM Spec Rendering

When creating a VM, merge Template + InstanceSize.spec_overrides:

```go
func RenderVMSpec(template *Template, instanceSize *InstanceSize, userParams map[string]interface{}) ([]byte, error) {
    // Start with template base
    vmSpec := template.BaseSpec
    
    // Apply InstanceSize overrides (JSON path → value)
    for path, value := range instanceSize.SpecOverrides {
        setValueAtPath(vmSpec, path, value)  // Generic JSON path setter
    }
    
    // Apply user-provided overrides (e.g., disk size)
    for path, value := range userParams {
        setValueAtPath(vmSpec, path, value)
    }
    
    return yaml.Marshal(vmSpec)
}
```

---

### Cluster Capability Matching

> **Key Principle**: Cluster capabilities are **auto-detected**, NOT manually configured by Admin.

**Auto-Detection Mechanism** (see ADR-0014 for details):

When Admin registers a cluster (provides kubeconfig only), the system automatically detects:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  System Auto-Detection (No manual input required)                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. GPU Devices:                                                             │
│     kubectl get nodes -o jsonpath='{.items[*].status.capacity}'              │
│     → Detect: nvidia.com/gpu, nvidia.com/GA102GL_A10, etc.                   │
│                                                                              │
│  2. Hugepages:                                                               │
│     kubectl get nodes -o jsonpath='{.items[*].status.allocatable}'           │
│     → Detect: hugepages-2Mi, hugepages-1Gi                                   │
│                                                                              │
│  3. SR-IOV Networks:                                                         │
│     kubectl get network-attachment-definitions -A                            │
│     → Detect: sriov-net-1, sriov-net-2                                       │
│                                                                              │
│  4. StorageClasses:                                                          │
│     kubectl get storageclasses                                               │
│     → Detect: ceph-rbd, local-path, etc.                                     │
│                                                                              │
│  5. KubeVirt Version:                                                        │
│     kubectl get kubevirt -n kubevirt                                         │
│     → Detect: v1.2.0                                                         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Storage Format**:

```go
// Cluster detected capabilities (auto-populated, read-only for Admin)
type Cluster struct {
    // ... other fields
    DetectedCapabilities DetectedCapabilities `json:"detected_capabilities"`
}

type DetectedCapabilities struct {
    GPUDevices      []string `json:"gpu_devices"`       // ["nvidia.com/GA102GL_A10"]
    Hugepages       []string `json:"hugepages"`         // ["2Mi", "1Gi"]
    SRIOVNetworks   []string `json:"sriov_networks"`    // ["sriov-net-1"]
    StorageClasses  []string `json:"storage_classes"`   // ["ceph-rbd"]
    KubeVirtVersion string   `json:"kubevirt_version"` // "v1.2.0"
}
```

**Matching Logic**:

```go
// Extract required resources from InstanceSize.spec_overrides
func ExtractRequiredResources(specOverrides map[string]interface{}) RequiredResources {
    var required RequiredResources
    
    // Extract GPU device names
    if gpus, ok := specOverrides["spec.template.spec.domain.devices.gpus"].([]interface{}); ok {
        for _, gpu := range gpus {
            if g, ok := gpu.(map[string]interface{}); ok {
                if deviceName, ok := g["deviceName"].(string); ok {
                    required.GPUDevices = append(required.GPUDevices, deviceName)
                }
            }
        }
    }
    
    // Extract hugepages requirement
    if pageSize, ok := specOverrides["spec.template.spec.domain.memory.hugepages.pageSize"].(string); ok {
        required.Hugepages = pageSize
    }
    
    return required
}

// Match InstanceSize requirements against cluster detected capabilities
func GetEligibleClusters(instanceSize *InstanceSize, environment string) []Cluster {
    required := ExtractRequiredResources(instanceSize.SpecOverrides)
    
    var eligible []Cluster
    for _, cluster := range GetClustersByEnvironment(environment) {
        if cluster.DetectedCapabilities.SupportsAll(required) {
            eligible = append(eligible, cluster)
        }
    }
    return eligible
}
```

---

## User Interaction Flow

### Role Definitions

| Role | Responsibility | Layer |
|------|---------------|-------|
| **Developer** | Define Mask (which Schema paths to expose) | Code/Config |
| **Platform Admin** | Create InstanceSizes, configure clusters | Admin UI |
| **End User** | Select InstanceSize, submit VM requests | User UI |

### Flow Diagram

```
Phase 0: Platform Setup (Developer)
┌─────────────────────────────────────────────────────────────────────────────┐
│  Developer:                                                                  │
│  1. Obtain KubeVirt JSON Schema (from CRD or official docs)                  │
│  2. Create Mask configuration specifying which paths to expose               │
│  3. Frontend reads Schema + Mask and auto-renders UI                         │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
Phase 1: Admin Configuration
┌─────────────────────────────────────────────────────────────────────────────┐
│  Platform Admin:                                                             │
│  1. Register clusters (provide kubeconfig only)                              │
│     - System AUTO-DETECTS capabilities (GPU, Hugepages, SR-IOV, etc.)        │
│     - Admin does NOT manually configure capabilities                         │
│  2. Create InstanceSizes via Admin UI (Schema-driven form)                   │
│     - Fill in values based on Schema types                                   │
│     - Values stored as spec_overrides (generic JSON)                         │
│  3. Create Templates (cloud-init, base image)                                │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
Phase 2: User Request
┌─────────────────────────────────────────────────────────────────────────────┐
│  End User:                                                                   │
│  1. Create System/Service (if not exists)                                    │
│  2. Submit VM request:                                                       │
│     - Select Namespace, Template, InstanceSize                               │
│     - Optionally override disk size (from quick_fields)                      │
│  3. Request enters approval queue                                            │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
Phase 3: Admin Approval
┌─────────────────────────────────────────────────────────────────────────────┐
│  Platform Admin:                                                             │
│  1. View pending request                                                     │
│  2. System auto-filters eligible clusters                                    │
│  3. Admin selects cluster, storage class                                     │
│  4. Approve request                                                          │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
Phase 4: VM Creation (Automatic)
┌─────────────────────────────────────────────────────────────────────────────┐
│  System:                                                                     │
│  1. Generate VM name: {namespace}-{system}-{service}-{index}                 │
│  2. Merge Template + InstanceSize.spec_overrides + user params               │
│  3. Render final VirtualMachine YAML                                         │
│  4. Apply to selected cluster                                                │
│  5. Notify user                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## API Design

### InstanceSize Management

```
GET  /api/v1/admin/instance-sizes           # List all
POST /api/v1/admin/instance-sizes           # Create
GET  /api/v1/admin/instance-sizes/{name}    # Get one
PUT  /api/v1/admin/instance-sizes/{name}    # Update
DELETE /api/v1/admin/instance-sizes/{name}  # Delete

GET  /api/v1/instance-sizes                 # List enabled (user-facing)
```

### Mask & Schema

```
GET /api/v1/schema                          # Get KubeVirt JSON Schema
GET /api/v1/mask                            # Get Mask configuration
```

---

### Canonical Interaction Flow Document (Post-Acceptance)

> **Decision**: Upon acceptance of this ADR, a standalone **Canonical Interaction Flow Document** SHALL be created as the single source of truth for all development teams.

**Rationale**:
- **Prevent Implementation Drift**: Without a unified flow document, frontend, backend, and database developers may have different interpretations of the same workflow
- **Enable Parallel Development**: Teams can develop and test against the same documented flows
- **Simplify Onboarding**: New team members have one authoritative reference
- **Support Audit**: Clear traceability from design decision to implementation

**Document Location**: `docs/design/interaction-flows/master-flow.md`

**Document Structure**:

| Section | Content |
|---------|---------|
| **Part 1: Platform Initialization** | Developer setup (Schema, Mask), RBAC/Permissions, OIDC/LDAP auth, IdP group mapping, Admin setup (Cluster, InstanceSize, Template) |
| **Part 2: Resource Management** | System/Service CRUD operations with database transactions and **audit logs** |
| **Part 3: VM Lifecycle** | Request → Approval → Execution → Deletion with state transitions and **audit logs** |
| **Part 4: State Machines & Data Model** | ApprovalTicket states, VM states, table relationships, **audit log design & exceptions** |

**Synchronization Rules**:
1. Any workflow change MUST be documented in the canonical flow document first
2. PRs modifying workflow logic MUST reference the relevant section of the flow document
3. Flow document updates MUST go through the same review process as code changes

> **Note**: The Chinese appendix (附录) in this ADR serves as the draft for this document. Upon ADR acceptance, it will be extracted and formalized as the canonical flow document.

---

## Consequences

### Positive

- ✅ **Schema as Truth**: Field types and options come from official KubeVirt Schema
- ✅ **Dumb Backend**: Backend only stores/retrieves JSON, no semantic interpretation
- ✅ **Auto-Updating**: When KubeVirt adds new fields, just update Schema + Mask
- ✅ **Flexible**: Users can fill any valid value, not limited to predefined options
- ✅ **Consistent**: UI rendering is automatic based on Schema types

### Negative

- 🟡 **Schema Complexity**: Frontend must handle complex JSON Schema parsing
- 🟡 **Matching Challenges**: Cluster capability matching needs resource extraction logic
- 🟡 **Validation**: Backend should validate against Schema before saving

### Mitigation

- Use existing JSON Schema UI libraries (e.g., react-jsonschema-form)
- Resource extraction can be implemented incrementally for common patterns
- Schema validation can be added as a middleware

---

## References

- [KubeVirt API Reference](https://kubevirt.io/api-reference/)
- [JSON Schema](https://json-schema.org/)
- [ADR-0015: Governance Model V2](./ADR-0015-governance-model-v2.md)
- [ADR-0017: VM Request Flow](./ADR-0017-vm-request-flow-clarification.md)

---

## Changelog

| Date | Change |
|------|--------|
| 2026-01-26 | Updated: Deprecated Decisions table now includes **Source ADR/Document** column for traceability |
| 2026-01-26 | Updated: Documents Requiring Updates section now follows **ADR Immutability Principle** - append-only amendments for accepted ADRs |
| 2026-01-26 | Added: Pre-written "Amendments by Subsequent ADRs" blocks for ADR-0015 and ADR-0014 (to be appended upon ADR-0018 acceptance) |
| 2026-01-26 | Added: Configuration Storage Strategy - PostgreSQL-first design with sensitive data encryption |
| 2026-01-26 | Added: First Deployment Bootstrap flow (admin/admin + force password change) |
| 2026-01-26 | Added: auth_providers and external_approval_systems table designs |
| 2026-01-26 | Added: Dual-layer permission model (Global RBAC + Resource-level RBAC) with inheritance |
| 2026-01-26 | Added: Stage 2.A+ Custom Role Management for platform administrators |
| 2026-01-26 | Added: Stage 4.A+ Resource-level member management (Owner adds members to System) |
| 2026-01-26 | Added: Permission boundary clarification (Shepherd controls visibility, Bastion controls SSH access) |
| 2026-01-26 | Added: Permission inheritance model (Service/VM fully inherit System permissions) |
| 2026-01-26 | Updated: Hugepages detection to include "None" as default option |
| 2026-01-26 | Updated: Detection methods for GPU, Hugepages, SR-IOV, KubeVirt version |
| 2026-01-26 | Updated: Overcommit+DedicatedCPU conflict warning upgraded to red color |
| 2026-01-26 | Added: Audit log JSON export section |
| 2026-01-26 | Added: External approval system integration section |
| 2026-01-24 | Added: Complete admin operation audit logs (Cluster/Template/InstanceSize/RBAC CRUD) with exceptions |
| 2026-01-24 | Added: Comprehensive audit log INSERT statements for all delete workflows (VM/Service) |
| 2026-01-24 | Restructured: Renumbered stages to sequential 1-5 (was 0/0.5/1.5/2/3), added navigation links between stages |
| 2026-01-24 | Added: Platform security configuration flow (RBAC, OIDC/LDAP, IdP group mapping, user login) |
| 2026-01-24 | Restructured: Merged "补充流程" sections into unified Part 1-4 structure |
| 2026-01-24 | Added: Decision for Canonical Interaction Flow Document (Post-Acceptance) |
| 2026-01-24 | Updated: Chinese appendix now serves as draft for master-flow.md |
| 2026-01-22 | Added: Complete user flows for System/Service creation and deletion |
| 2026-01-22 | Added: Detailed database operations (INSERT/UPDATE/DELETE) for VM lifecycle |
| 2026-01-22 | Added: State transition diagrams for ApprovalTicket and VM |
| 2026-01-22 | Added: Core database table relationship overview diagram |
| 2026-01-22 | Updated: Administrator approval UI with disk size modification option |
| 2026-01-22 | Updated: Overcommit parameter display logic (shown when enabled, regardless of environment) |
| 2026-01-22 | Updated: Template configuration UI to form-based approach (removed YAML editor) |
| 2026-01-22 | Updated: Cloud-init example to use one-time password instead of Go Template variables |
| 2026-01-22 | Added: Amendment Notice and Design Changes Summary |
| 2026-01-22 | Added: Documents Requiring Updates section |
| 2026-01-22 | Added: Namespace and Template configuration steps in Chinese workflow (ADR-0015 §5, §9, §17) |
| 2026-01-22 | Added: Conflict warning for Dedicated CPU + Overcommit configuration |
| 2026-01-22 | Integrated: Overcommit settings into InstanceSize creation and Approval flows |
| 2026-01-22 | Added: Overcommit Configuration (Request/Limit Model) with UI mockups for Admin |
| 2026-01-22 | Major rewrite: Schema-driven design, removed predefined capability options |
| 2026-01-22 | Previous: Generic KV matching (superseded) |

---

## Documents Requiring Updates

> This section lists documents that must be updated to reflect the decisions in this ADR.
>
> **ADR Immutability Principle**: Per industry best practices, once an ADR is **Accepted**, its original content should remain unchanged to preserve historical context. Amendments are handled by:
> 1. **Appending** a read-only "Amendments by Subsequent ADRs" section at the end of the affected ADR
> 2. **Never modifying** the original decision text, code blocks, or rationale
> 3. **Linking bidirectionally** between the original and amending ADRs

---

### 1. ADR-0015: Governance Model V2 (Accepted)

**File**: `docs/adr/ADR-0015-governance-model-v2.md`  
**Status**: Accepted (Immutable)  
**Action**: **APPEND** section at end of file (do NOT modify original content)

**Sections Affected**:
- §5. Template Layered Design (lines 240-310): `required_features`, `required_hardware` fields
- §5. Template Schema (lines 252-279): Hardware capability definitions

**Amendment Block to Append** (at end of ADR-0015, after the last section):

```markdown
---

## Amendments by Subsequent ADRs

> ⚠️ **Notice**: The following sections of this ADR have been amended by subsequent ADRs.
> The original decisions above remain **unchanged for historical reference**.
> When implementing, please refer to the amending ADRs for current design.

### ADR-0018: Instance Size Abstraction (2026-01-22)

| Original Section | Status | Amendment Details | See Also |
|------------------|--------|-------------------|----------|
| §5. Template Layered Design: `required_features`, `required_hardware` | **MOVED** | Capability requirements now defined in InstanceSize, not Template | [ADR-0018 §4](./ADR-0018-instance-size-abstraction.md#4-backend-storage-dumb) |
| §5. Template Layered Design: Hardware capability definitions | **MOVED** | GPU/SR-IOV/Hugepages capabilities configured via InstanceSize | [ADR-0018 §Cluster Capability Matching](./ADR-0018-instance-size-abstraction.md#cluster-capability-matching) |
| §5. Template Schema: `field.Strings("required_features")` | **SUPERSEDED** | Use InstanceSize.spec_overrides instead | [ADR-0018 InstanceSize Schema](./ADR-0018-instance-size-abstraction.md#4-backend-storage-dumb) |
| §5. Template Schema: `field.Strings("required_hardware")` | **SUPERSEDED** | Use InstanceSize.spec_overrides instead | [ADR-0018 InstanceSize Schema](./ADR-0018-instance-size-abstraction.md#4-backend-storage-dumb) |

> **Implementation Guidance**: Template retains `quick_fields` and `advanced_fields` for UI field visibility control. All hardware capability requirements (GPU, SR-IOV, Hugepages, dedicated CPU) are now configured in InstanceSize and matched against cluster detected capabilities.

---
```

---

### 2. ADR-0014: Capability Detection (Accepted)

**File**: `docs/adr/ADR-0014-capability-detection.md`  
**Status**: Accepted (Immutable)  
**Action**: **APPEND** section at end of file (do NOT modify original content)

**Sections Affected**:
- Template `required_features` metadata (lines 42, 117, 205, 210)

**Amendment Block to Append**:

```markdown
---

## Amendments by Subsequent ADRs

> ⚠️ **Notice**: Partial amendments to this ADR by subsequent ADRs.

### ADR-0018: Instance Size Abstraction (2026-01-22)

| Original Section | Status | Amendment Details | See Also |
|------------------|--------|-------------------|----------|
| Template `required_features` metadata | **MOVED** | Feature requirements now stored in InstanceSize.spec_overrides, not Template | [ADR-0018](./ADR-0018-instance-size-abstraction.md) |

> **Note**: The cluster capability detection mechanism described in this ADR remains valid. The change only affects WHERE requirements are stored (InstanceSize instead of Template).

---
```

---

### 3. `docs/design/phases/01-contracts.md` (Not an ADR, Can Modify)

**File**: `docs/design/phases/01-contracts.md`  
**Status**: Design document (Mutable)  
**Action**: Directly modify content

| Section | Current State | Required Change |
|---------|---------------|-----------------|
| **NEW**: InstanceSize Schema | Not exists | **ADD**: Complete InstanceSize schema with `spec_overrides`, `cpu_overcommit`, `mem_overcommit` |
| **NEW**: InstanceSize Indexes | Not exists | **ADD**: `index.Fields("name").Unique()` |
| **NEW**: resource_role_bindings table | Not exists | **ADD**: Resource-level RBAC table |
| **NEW**: users table | Not exists | **ADD**: With `force_password_change` flag |
| **NEW**: auth_providers table | Not exists | **ADD**: OIDC/LDAP configuration storage |
| **NEW**: external_approval_systems table | Not exists | **ADD**: External approval integration |
| **Reference**: ADR-0018 | Not exists | **ADD**: Reference to this ADR for InstanceSize design |

---

### 4. `docs/design/phases/03-service-layer.md` (Not an ADR, Can Modify)

**File**: `docs/design/phases/03-service-layer.md`  
**Status**: Design document (Mutable)  
**Action**: Directly modify content

| Section | Current State | Required Change |
|---------|---------------|-----------------|
| Approval Flow | Basic approval fields | **ADD**: Overcommit override fields in approval request |
| Approval Warnings | Not exists | **ADD**: Production overcommit warning, Dedicated CPU conflict warning |
| **NEW**: InstanceSize Management | Not exists | **ADD**: Admin InstanceSize CRUD operations |

---

### 5. `docs/design/phases/04-governance.md` (Not an ADR, Can Modify)

**File**: `docs/design/phases/04-governance.md`  
**Status**: Design document (Mutable)  
**Action**: Directly modify content

| Section | Current State | Required Change |
|---------|---------------|-----------------|
| Approval Matrix | Missing overcommit handling | **ADD**: Overcommit approval with environment-aware warnings |
| **NEW**: Admin Configuration Workflow | Not exists | **ADD**: Namespace, Template, InstanceSize configuration sequence |

---

### 6. `docs/design/examples/domain/` (Not an ADR, Can Modify)

**File**: `docs/design/examples/domain/`  
**Status**: Example code (Mutable)  
**Action**: Create new files or modify existing

| File | Current State | Required Change |
|------|---------------|-----------------|
| **NEW**: `instance_size.go` | Not exists | **ADD**: InstanceSize entity with all fields |
| **NEW**: `overcommit.go` | Not exists | **ADD**: OvercommitConfig struct |
| **NEW**: `resource_role_binding.go` | Not exists | **ADD**: ResourceRoleBinding entity |
| **NEW**: `permission.go` | Not exists | **ADD**: Permission check logic with inheritance |

### 6. New Files to Create

| File Path | Content |
|-----------|---------|
| `config/mask.yaml` | Mask configuration defining exposed Schema paths |
| `config/seed/templates.yaml` | Pre-populated template data for system initialization |
| `config/seed/instance_sizes.yaml` | Pre-populated InstanceSize data for system initialization |

### 7. Permission System Updates (ADR-0018 §Stage 2.A+, §Stage 4.A+)

> **Added 2026-01-26**: Dual-layer permission model design

| Document | Section | Required Change |
|----------|---------|-----------------|
| `docs/design/phases/01-contracts.md` | Database Schema | **ADD**: `resource_role_bindings` table for resource-level RBAC |
| `docs/design/phases/04-governance.md` | Permission Model | **ADD**: Dual-layer permission design (Global RBAC + Resource-level RBAC) |
| `docs/design/phases/04-governance.md` | Role Management | **ADD**: Custom role management workflow for platform administrators |
| `docs/design/examples/domain/` | **NEW**: `resource_role_binding.go` | **ADD**: ResourceRoleBinding entity with fields (user_id, role, resource_type, resource_id, granted_by) |
| `docs/design/examples/domain/` | **NEW**: `permission.go` | **ADD**: Permission check logic with inheritance |

**New Database Tables**:

```sql
-- Global RBAC (OIDC/LDAP group mapping)
role_bindings (id, user_id, role_id, scope_type, allowed_environments, source)

-- Resource-level RBAC (User self-service)
resource_role_bindings (id, user_id, role, resource_type, resource_id, granted_by, created_at)
```

**Supersedes** in ADR-0015:
- §2 RoleBinding: Add resource-level binding supplement (not replacement)

### 8. Configuration Storage Tables (ADR-0018 §Configuration Storage Strategy)

> **Added 2026-01-26**: PostgreSQL-first configuration storage

| Document | Section | Required Change |
|----------|---------|-----------------|
| `docs/design/phases/01-contracts.md` | Database Schema | **ADD**: `users` table with `force_password_change` flag |
| `docs/design/phases/01-contracts.md` | Database Schema | **ADD**: `auth_providers` table for OIDC/LDAP config |
| `docs/design/phases/01-contracts.md` | Database Schema | **ADD**: `external_approval_systems` table |
| `docs/design/phases/00-prerequisites.md` | Bootstrap | **ADD**: First deployment flow with admin/admin seed |

**New Database Tables**:

```sql
-- Local users (JWT authentication)
CREATE TABLE users (
    id VARCHAR(36) PRIMARY KEY,
    username VARCHAR(100) NOT NULL UNIQUE,
    email VARCHAR(255),
    password_hash VARCHAR(255) NOT NULL,    -- bcrypt
    display_name VARCHAR(100),
    auth_type VARCHAR(20) NOT NULL,         -- 'local', 'oidc', 'ldap'
    external_id VARCHAR(255),               -- OIDC sub or LDAP DN
    force_password_change BOOLEAN DEFAULT FALSE,
    last_login_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- OIDC/LDAP providers
CREATE TABLE auth_providers (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,             -- "企业 SSO", "Azure AD"
    type VARCHAR(20) NOT NULL,              -- 'oidc', 'ldap'
    priority INT NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    
    -- OIDC config
    oidc_issuer_url TEXT,
    oidc_client_id TEXT,
    oidc_client_secret TEXT,                -- AES-256-GCM encrypted
    oidc_scopes TEXT[],
    
    -- LDAP config
    ldap_host TEXT,
    ldap_port INT,
    ldap_use_tls BOOLEAN DEFAULT TRUE,
    ldap_bind_dn TEXT,
    ldap_bind_password TEXT,                -- AES-256-GCM encrypted
    ldap_user_search_base TEXT,
    ldap_user_filter TEXT,
    ldap_group_search_base TEXT,
    
    created_by VARCHAR(36) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- External approval systems
CREATE TABLE external_approval_systems (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,             -- "OA 审批", "ServiceNow"
    type VARCHAR(50) NOT NULL,              -- 'webhook', 'servicenow', 'jira'
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    
    -- Webhook config
    webhook_url TEXT,
    webhook_secret TEXT,                    -- AES-256-GCM encrypted
    webhook_headers JSONB,
    
    -- Common config
    timeout_seconds INT DEFAULT 30,
    retry_count INT DEFAULT 3,
    
    created_by VARCHAR(36) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

---

## Appendix: Canonical Interaction Flows

> **Note**: The detailed interaction flows have been extracted to a standalone document.
> 
> **Canonical Version (English)**: [master-flow.md](../design/interaction-flows/master-flow.md)
> 
> **Chinese Translation**: [i18n/zh-CN/design/interaction-flows/master-flow.md](../i18n/zh-CN/design/interaction-flows/master-flow.md)
>
> **2026-01-26 Update**: Added Configuration Storage Strategy, First Deployment Bootstrap, External Approval Systems.
> The YAML configuration example has been removed - all runtime config uses Web UI + PostgreSQL.

### Document Structure

| Part | Content | Involved Roles |
|------|---------|----------------|
| **Part 1** | Platform Initialization (Schema/Mask, Bootstrap, RBAC, Auth, External Approvals, Cluster/InstanceSize/Template) | Developer, Platform Admin |
| **Part 2** | Resource Management (System/Service CRUD with audit logs) | Regular User |
| **Part 3** | VM Lifecycle (Request → Approval → Execution → Delete with audit logs) | Regular User, Platform Admin |
| **Part 4** | State Machines & Data Models (State transitions, Table relationships, Audit log design) | All Developers |

---

### Key Design Principles

| Principle | Description |
|-----------|-------------|
| **Schema as Single Source of Truth** | KubeVirt official JSON Schema defines all field types, constraints, enum options |
| **Mask Only Selects Paths** | Mask specifies which Schema paths to expose, does NOT define field options |
| **Dumb Backend** | Backend stores `map[string]interface{}` without interpreting field semantics |
| **Schema-Driven Frontend** | Frontend reads JSON Schema + Mask to render appropriate UI components |

### Role Definitions

| Role | Responsibility | Layer |
|------|----------------|-------|
| **Developer** | Obtain KubeVirt Schema, define Mask (select exposed paths) | Code/Config |
| **Platform Admin** | Create InstanceSize (Schema-driven forms), configure RBAC | Admin Backend |
| **Regular User** | Select InstanceSize, submit VM creation requests | Business Usage |


---

> **Full Details**: The complete interaction flows with all database operations, UI mockups, and detailed state machine diagrams have been moved to [master-flow.md](../design/interaction-flows/master-flow.md) to keep this ADR concise per CNCF best practices.

---

### Configuration Storage Strategy (Summary)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Layer 1: Infrastructure Config (config.yaml OR env vars)                   │
│  - DATABASE_URL, SERVER_PORT, LOG_LEVEL, ENCRYPTION_KEY                     │
│  - Priority: Environment Variables > config.yaml > defaults                │
├─────────────────────────────────────────────────────────────────────────────┤
│  Layer 2: PostgreSQL (All Runtime Configuration via Web UI)                 │
│  - users, auth_providers, external_approval_systems                        │
│  - roles, role_bindings, resource_role_bindings                            │
│  - clusters, instance_sizes, templates                                      │
│  - All business data + audit_logs                                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

### State Machine Summary

**ApprovalTicket States**:
```
PENDING_APPROVAL → APPROVED → EXECUTING → SUCCESS/FAILED
                 → REJECTED (terminal)
                 → CANCELLED (terminal)
```

**VM States**:
```
CREATING → RUNNING ↔ STOPPED
         → FAILED (terminal)
RUNNING/STOPPED → DELETING → DELETED (terminal)
```

---

_End of ADR-0018_
