# ADR-0015: Governance Model V2 - Decoupled Hierarchy and Enhanced Controls

> **Status**: Accepted  
> **Date**: 2026-01-21 (Accepted)  
> **Supersedes**: Portions of Phase 1 Contracts (01-contracts.md) related to governance model

---

## Context

### Problem Statement

The initial governance model design had several limitations that would impact long-term scalability and operational flexibility:

1. **Tight Coupling**: System was bound to namespace, environment, and cluster, which would cause management complexity when scaling across multiple namespaces or clusters
2. **Single Maintainer**: Only `created_by` field existed, but real-world scenarios require team-based ownership
3. **Insufficient Field Controls**: Users could customize VM names, cloud-init, and labels, which would break platform governance and traceability
4. **Coarse Template Configuration**: No distinction between quick/standard fields and advanced features requiring special hardware
5. **Missing Audit Trail**: Modification and deletion operations lacked proper tracking
6. **Rigid Approval Policies**: No differentiation between test and production environments
7. **Storage Class Management**: No automated detection or admin-controlled defaults for cluster storage
8. **Unclear Namespace Responsibility**: Ambiguity about whether the platform should manage Kubernetes RBAC/ResourceQuota

### Design Goals

1. Decouple governance entities from infrastructure concerns
2. Support team-based ownership model
3. Enforce strict field controls to maintain platform governance
4. Enable template-driven field visibility (quick vs. advanced)
5. Provide comprehensive operation audit trail
6. Support environment-aware approval policies with future configurability
7. Automate storage class detection with admin override capability
8. Clarify platform responsibility boundaries for Kubernetes resources

---

## Decision

### 1. System Entity Decoupling

**Decision**: Remove namespace, environment, and cluster bindings from System entity. Permissions are managed via separate RBAC tables (see [§22. Authentication & RBAC Strategy](#22-authentication--rbac-strategy)).

**Rationale**:
- A System represents a logical business grouping, not an infrastructure deployment unit
- VMs under a System may span multiple clusters and namespaces
- Team ownership is managed via RoleBinding, not stored in entity itself
- RBAC tables enable OIDC/LDAP integration and fine-grained permission control

**Schema Changes**:

```go
// ent/schema/system.go
func (System) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("name").NotEmpty(),
        field.String("description").Optional(),
        field.String("created_by").NotEmpty(),
        // NOTE: No maintainers field - permissions managed via RoleBinding table
        field.String("tenant_id").Default("default").Immutable(),  // Multi-tenancy reserved
        field.Time("created_at").Default(time.Now).Immutable(),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

func (System) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("name").Unique(),  // Globally unique
    }
}
```

---

### 2. Service Entity and Permission Inheritance

**Decision**: Service does NOT store its own `maintainers`. Permissions are fully inherited from parent System at runtime via edge query. The `name` field is immutable after creation.

**Rationale**:
- Service name is used as part of VM hostname generation
- Changing Service name would break naming consistency and traceability
- **Runtime inheritance** ensures permission changes on System immediately affect all child Services
- Simplifies permission management: administrators only manage System-level permissions
- Reduces data duplication and synchronization complexity

**Permission Resolution Pattern**:

```go
// Permission check: Service inherits from System via RBAC
func (s *PermissionService) CanAccessService(ctx context.Context, userID, serviceID string) (bool, error) {
    service, _ := s.serviceRepo.Query(ctx).Where(service.IDEQ(serviceID)).WithSystem().Only(ctx)
    system := service.Edges.System
    
    // Check against RBAC tables (see §22 for full RBAC schema)
    return s.hasPermission(ctx, userID, "service:read", "system", system.ID)
}
```

**Schema Changes**:

```go
// ent/schema/service.go
func (Service) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("name").NotEmpty().Immutable(),           // Cannot change after creation
        field.String("description").Optional(),
        field.Int("next_instance_index").Default(1),
        field.Time("created_at").Default(time.Now).Immutable(),
        // NOTE: No created_by, no maintainers - fully inherited from System
    }
}

func (Service) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("system", System.Type).Ref("services").Unique().Required(),
        edge.To("vms", VM.Type),
    }
}
```

**Permission Inheritance Matrix**:

| Entity | Permission Source | Storage |
|--------|------------------|--------|
| System | RoleBinding with scope_type="system" | RoleBinding table |
| Service | Inherited from parent System | RoleBinding table (query via System) |
| VM | Inherited via Service → System chain | RoleBinding table (query via System) |

---

### 3. VM Entity Association Strategy

**Decision**: VM only associates with `service_id`. System information is obtained through Service relationship (Service → System).

**Rationale**:
- Single Source of Truth: Avoids data inconsistency between `vm.system_id` and `service.system_id`
- Semantic Correctness: VM → Service → System is the natural hierarchy
- Query Efficiency: Ent ORM's Eager Loading handles relationship queries efficiently

**Schema Changes**:

```go
// ent/schema/vm.go
func (VM) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("service", Service.Type).Ref("vms").Unique().Required(),
        // NO direct system_id - obtained via service.system_id
        edge.To("revisions", VMRevision.Type),
    }
}
```

**Query Pattern**:

```go
vm, _ := client.VM.Query().
    Where(vm.IDEQ(vmID)).
    WithService(func(q *ent.ServiceQuery) {
        q.WithSystem()
    }).
    Only(ctx)

systemName := vm.Edges.Service.Edges.System.Name
```

---

### 4. VM Field Control Enforcement

**Decision**: Strictly control user-modifiable fields. The following are platform-controlled and user-forbidden:

| Field | Control | Rationale |
|-------|---------|-----------|
| `name` | **Platform-generated** | Format: `{namespace}-{system}-{service}-{index}` for cluster-wide uniqueness |
| `cloud_init` | **Template-defined only** | Security-sensitive configuration |
| `labels` | **Platform-managed only** | Governance labels must not be tampered |

**VM Name Generation**:

> **See Also**: [§16. Global Unique Naming](#16-global-unique-naming-and-vm-name-format) for complete naming constraints and RFC 1123 compliance rules.

```go
// VM name includes namespace prefix for cluster-wide uniqueness
// All components have length limits: namespace ≤15, system ≤15, service ≤15
func GenerateVMName(namespace, systemName, serviceName string, index int) string {
    return fmt.Sprintf("%s-%s-%s-%02d", namespace, systemName, serviceName, index)
}

// Example: dev-shop-redis-01 (max 50 chars, well within RFC 1123 limit of 63)
```

**Platform-Managed Labels** (automatically applied, user cannot modify):

```yaml
labels:
  kubevirt-shepherd.io/managed-by: kubevirt-shepherd
  kubevirt-shepherd.io/system: {{ .SystemName }}
  kubevirt-shepherd.io/service: {{ .ServiceName }}
  kubevirt-shepherd.io/instance: {{ .InstanceIndex }}
  kubevirt-shepherd.io/ticket-id: {{ .TicketID }}
  kubevirt-shepherd.io/created-by: {{ .CreatedBy }}
  kubevirt-shepherd.io/hostname: {{ .VMName }}
```

**User-Submittable Fields**:

```go
type VMCreateRequest struct {
    ServiceID   string `json:"service_id" binding:"required"`
    TemplateID  string `json:"template_id" binding:"required"`
    ClusterID   string `json:"cluster_id" binding:"required"`
    Namespace   string `json:"namespace" binding:"required"`
    
    // Quick mode adjustable fields (controlled by template mask)
    CPU       *int `json:"cpu,omitempty"`
    MemoryMB  *int `json:"memory_mb,omitempty"`
    DiskGB    *int `json:"disk_gb,omitempty"`
    
    // Advanced mode fields (visible only if template enables)
    GPU       *int    `json:"gpu,omitempty"`
    Hugepages *string `json:"hugepages,omitempty"`
    NUMA      *string `json:"numa,omitempty"`
    
    Reason string `json:"reason" binding:"required"`
    
    // FORBIDDEN fields - not accepted from user input:
    // Name, CloudInit, Labels
}
```

---

### 5. Template Layered Design (Quick / Advanced)

**Decision**: Templates define two layers of field visibility controlled by frontend mask configuration.

**Rationale**:
- Quick mode: Common fields (CPU, memory, disk, image) for standard use cases
- Advanced mode: Hardware-dependent features (GPU, Hugepages, NUMA, SR-IOV) requiring capability detection (ADR-0014)
- Frontend mask allows runtime control of field visibility

**Schema Changes**:

```go
// ent/schema/template.go
func (Template) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("name").NotEmpty(),
        field.Int("version").Default(1),
        field.Text("content"),
        field.Enum("status").Values("draft", "active", "deprecated", "archived"),
        field.String("category").Optional(),
        
        // Capability requirements (ADR-0014)
        field.Strings("required_features").Optional(),
        field.Strings("required_hardware").Optional(),
        
        // Field visibility control (frontend mask)
        field.JSON("quick_fields", QuickFields{}).
            Comment("Quick mode field configuration"),
        field.JSON("advanced_fields", AdvancedFields{}).
            Comment("Advanced mode field configuration"),
        field.JSON("field_defaults", map[string]interface{}{}).
            Comment("Default values for all fields"),
        field.JSON("field_constraints", map[string]Constraint{}).
            Comment("Field constraints (min/max/options)"),
            
        field.String("created_by").NotEmpty(),
        field.Time("created_at").Default(time.Now),
    }
}
```

**Field Configuration Structure**:

```go
type QuickFields struct {
    CPU       FieldConfig `json:"cpu"`
    MemoryMB  FieldConfig `json:"memory_mb"`
    DiskGB    FieldConfig `json:"disk_gb"`
    Image     FieldConfig `json:"image"`
}

type AdvancedFields struct {
    GPU       FieldConfig `json:"gpu"`
    Hugepages FieldConfig `json:"hugepages"`
    NUMA      FieldConfig `json:"numa"`
    Network   FieldConfig `json:"network"`
    SRIOV     FieldConfig `json:"sriov"`
}

type FieldConfig struct {
    Visible   bool        `json:"visible"`
    Editable  bool        `json:"editable"`
    Required  bool        `json:"required"`
    Default   interface{} `json:"default"`
    Min       *int        `json:"min,omitempty"`
    Max       *int        `json:"max,omitempty"`
    Options   []string    `json:"options,omitempty"`
}
```

---

### 6. Comprehensive Operation Audit Trail

**Decision**: All user operations (create, modify, delete, power operations) are recorded via DomainEvent pattern.

**Extended Event Types**:

```go
const (
    // Creation
    EventVMCreationRequested  EventType = "VM_CREATION_REQUESTED"
    EventVMCreationCompleted  EventType = "VM_CREATION_COMPLETED"
    EventVMCreationFailed     EventType = "VM_CREATION_FAILED"
    
    // Modification
    EventVMModifyRequested    EventType = "VM_MODIFY_REQUESTED"
    EventVMModifyCompleted    EventType = "VM_MODIFY_COMPLETED"
    EventVMModifyFailed       EventType = "VM_MODIFY_FAILED"
    
    // Deletion
    EventVMDeletionRequested  EventType = "VM_DELETION_REQUESTED"
    EventVMDeletionCompleted  EventType = "VM_DELETION_COMPLETED"
    EventVMDeletionFailed     EventType = "VM_DELETION_FAILED"
    
    // Power Operations
    EventVMStartRequested     EventType = "VM_START_REQUESTED"
    EventVMStopRequested      EventType = "VM_STOP_REQUESTED"
    EventVMRestartRequested   EventType = "VM_RESTART_REQUESTED"
    
    // Cancellation
    EventRequestCancelled     EventType = "REQUEST_CANCELLED"
)
```

**User Query APIs**:

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/events?requested_by=me` | User's own events |
| `GET /api/v1/events?aggregate_type=vm&aggregate_id={id}` | Specific VM history |

---

### 7. Environment-Aware Approval Policies

**Decision**: Default approval policies differentiate between test and prod environments. Future versions will support admin-configurable policies via frontend.

**Default Policy Matrix**:

| Operation | test Environment | prod Environment |
|-----------|------------------|------------------|
| CREATE_VM | ✅ Requires Approval | ✅ Requires Approval |
| MODIFY_VM | ✅ Requires Approval | ✅ Requires Approval |
| DELETE_VM | ✅ Requires Approval | ✅ Requires Approval |
| START_VM | ❌ No Approval | ✅ Requires Approval |
| STOP_VM | ❌ No Approval | ✅ Requires Approval |
| RESTART_VM | ❌ No Approval | ✅ Requires Approval |

**Schema for Configurable Policies** (future):

```go
// ent/schema/approval_policy.go
func (ApprovalPolicy) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("name").NotEmpty(),
        field.Enum("environment").Values("test", "prod", "all"),
        field.Enum("operation").Values(
            "CREATE_VM", "MODIFY_VM", "DELETE_VM",
            "START_VM", "STOP_VM", "RESTART_VM",
        ),
        field.Bool("requires_approval").Default(true),
        field.Strings("approvers").Optional(),
        field.Int("priority").Default(0),
        field.Bool("enabled").Default(true),
        field.Time("created_at").Default(time.Now),
    }
}
```

---

### 8. Cluster Storage Class Management

**Decision**: Platform auto-detects StorageClasses during health check. Admin sets default. Approval workflow allows override selection.

**Rationale**:
- Auto-detection reduces manual configuration burden
- Admin-controlled default ensures operational consistency
- Per-approval override provides flexibility for special cases
- Real-time cluster query prevents cross-cluster SC mismatch

**Schema Changes**:

```go
// ent/schema/cluster.go - additional fields
field.Strings("storage_classes").Optional().
    Comment("Auto-detected StorageClass list"),
field.String("default_storage_class").Optional().
    Comment("Admin-specified default StorageClass"),
field.Time("storage_classes_updated_at").Optional(),
```

**Detection Logic** (during health check):

```go
func (d *CapabilityDetector) DetectStorageClasses(ctx context.Context, restConfig *rest.Config) ([]string, error) {
    clientset, _ := kubernetes.NewForConfig(restConfig)
    scList, _ := clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
    
    var names []string
    for _, sc := range scList.Items {
        names = append(names, sc.Name)
    }
    return names, nil
}
```

**Approval Workflow Enhancement**:

```go
// ApprovalTicket additional field
field.String("selected_storage_class").Optional().
    Comment("Admin-selected StorageClass during approval, empty uses cluster default"),
```

---

### 9. Namespace Responsibility Boundary

**Decision**: This platform does NOT manage Kubernetes RBAC or ResourceQuota. It MAY assist with namespace creation.

**Responsibility Matrix**:

| Capability | Platform Responsibility | Notes |
|------------|------------------------|-------|
| Create Namespace | ✅ Optional helper | Admin API for convenience |
| RBAC Configuration | ❌ Not managed | Kubernetes admin responsibility |
| ResourceQuota | ❌ Not managed | Kubernetes admin responsibility |
| Namespace Exists Check | ✅ Validation | Pre-creation verification |

**Helper API** (Admin only):

```bash
# Check namespace existence
GET /api/v1/clusters/{cluster_id}/namespaces/{name}/exists

# Create namespace (optional convenience)
POST /api/v1/admin/clusters/{cluster_id}/namespaces
{
  "name": "production",
  "labels": {
    "environment": "prod"
  }
}
```

---

### 10. User Request Cancellation and Duplicate Prevention

**Decision**: Users can cancel pending requests. Same resource with same operation type cannot have duplicate pending requests.

**Cancellation Mechanism**:

- Users may cancel requests in `PENDING_APPROVAL` status
- Cancellation reuses rejection logic flow (consistent audit trail)
- Cancelled requests are marked with `CANCELLED` status (not deleted)
- Audit record includes cancellation reason and timestamp

**Duplicate Request Prevention**:

```go
// Before accepting new request, check for existing pending
func (s *ApprovalService) ValidateNoDuplicate(ctx context.Context, 
    resourceID string, operationType string) error {
    
    exists, _ := s.ticketRepo.ExistsPending(ctx, resourceID, operationType)
    if exists {
        return &AppError{
            Code: "DUPLICATE_PENDING_REQUEST",
            Params: map[string]interface{}{
                "resource_id": resourceID,
                "operation":   operationType,
            },
        }
    }
    return nil
}
```

**User-Friendly Response**:

```json
{
  "code": "DUPLICATE_PENDING_REQUEST",
  "message": "A pending request already exists for this resource",
  "params": {
    "existing_ticket_id": "TKT-12345",
    "operation": "CREATE_VM"
  }
}
```

---

### 11. Approval Timeout Handling

**Decision**: V1 does not implement automatic timeout or escalation. UI-based prioritization is used instead.

**Rationale**:
- The duplicate request lock mechanism naturally encourages users to follow up with administrators
- Complex timeout/escalation logic adds maintenance burden without proportional benefit in V1

**UI Prioritization Strategy**:

| Days Pending | Visual Treatment | Sort Priority |
|--------------|-----------------|---------------|
| 0-3 days | Normal | Standard |
| 4-7 days | Yellow highlight | Higher |
| 7+ days | Red highlight | Highest (top) |

**Future Consideration**: Timeout auto-rejection or escalation may be added in later versions via RFC.

---

### 12. Resource Adoption Rules

**Decision**: Adoption is a compensation mechanism for resources created in K8s but not recorded in PostgreSQL. Only resources with existing Service association can be adopted.

**Adoption Criteria**:

| Condition | Adoptable | Action |
|-----------|-----------|--------|
| Has valid `kubevirt-shepherd.io/service` label AND Service exists in DB | ✅ Yes | Show in pending adoptions |
| Has Shepherd labels but Service not found | ❌ No | Ignore (orphan resource) |
| No Shepherd labels | ❌ No | Not platform-managed |

**Rationale**:
- Adoption is for recovery from rare failures (e.g., DB write failed after K8s create)
- Resources without valid Service relationship are orphans outside platform governance
- Admins can manually delete orphan resources via kubectl if needed

---

### 13. Deletion Cascade Constraints

**Decision**: Hard delete with strict cascade constraints. Only audit records are preserved.

**Cascade Rules**:

| Entity | Deletion Constraint | Data Retention |
|--------|---------------------|----------------|
| System | Must have zero Services | Hard delete, audit preserved |
| Service | Must have zero VMs | Hard delete, audit preserved |
| VM | Direct delete allowed | Hard delete, audit preserved |

**Rationale for Hard Delete**:
- Prevents naming conflicts with future entities of same name
- Simplifies data model (no soft-delete tombstones)
- Audit logs provide complete historical record
- Reduces storage complexity

**Implementation**:

```go
func (s *SystemService) Delete(ctx context.Context, systemID string) error {
    // Check cascade constraint
    serviceCount, _ := s.serviceRepo.CountBySystemID(ctx, systemID)
    if serviceCount > 0 {
        return &AppError{
            Code: "DELETE_RESTRICTED",
            Params: map[string]interface{}{
                "entity":       "system",
                "children":     "services",
                "child_count":  serviceCount,
            },
        }
    }
    
    // Record audit before delete
    s.auditLogger.Log(ctx, AuditSystemDeleted, systemID)
    
    // Hard delete
    return s.repo.Delete(ctx, systemID)
}
```

---

### 13.1 Delete Confirmation Mechanism

**Decision**: Implement tiered delete confirmation based on entity sensitivity. Users must type the resource name to confirm deletion.

> **Rationale**: Hard delete is irreversible. Requiring users to type the resource name forces deliberate confirmation, preventing accidental deletions. This follows the GitHub/AWS industry standard pattern.

**Tiered Confirmation Strategy**:

| Entity | Confirmation Method | Implementation |
|--------|--------------------|-----------------| 
| **VM** (test) | `confirm=true` query parameter | Lightweight, API-level protection |
| **VM** (prod) | Type VM name in request body | Strong confirmation for production resources |
| **Service** | `confirm=true` query parameter | Same as test VM (requires no child VMs) |
| **System** | Type system name in request body | Strong confirmation for root entity |

**API Design**:

```bash
# Test VM Delete - simple confirm parameter
DELETE /api/v1/vms/{id}?confirm=true

# Prod VM Delete - requires typing VM name
DELETE /api/v1/vms/{id}
Content-Type: application/json
{
  "confirm_name": "prod-shop-redis-01"  // Must match VM name exactly
}

# Service Delete - requires confirm parameter  
DELETE /api/v1/services/{id}?confirm=true

# System Delete - requires typing system name
DELETE /api/v1/systems/{id}
Content-Type: application/json
{
  "confirm_name": "shop"  // Must match system name exactly
}
```

**Error Responses**:

```go
// Missing or incorrect confirmation
type ErrDeleteConfirmationRequired struct {
    Entity       string `json:"entity"`        // "vm", "service", "system"
    EntityID     string `json:"entity_id"`
    EntityName   string `json:"entity_name"`   // Name user must type to confirm
    Environment  string `json:"environment,omitempty"`
    Message      string `json:"message"`       // Human-readable instruction
}

func (e *ErrDeleteConfirmationRequired) Code() string {
    return "DELETE_CONFIRMATION_REQUIRED"
}

// Name mismatch
type ErrConfirmationNameMismatch struct {
    Expected string `json:"expected"`  // Correct name (displayed in UI)
    Provided string `json:"provided"`  // What user typed
}

func (e *ErrConfirmationNameMismatch) Code() string {
    return "CONFIRMATION_NAME_MISMATCH"
}
```

**Frontend UX Guidelines**:

| Entity | Environment | Recommended UI Pattern |
|--------|-------------|------------------------|
| VM | test | Modal with "Delete" button, auto-adds `confirm=true` |
| VM | prod | Modal with **red highlighted VM name**, input field for user to type name |
| Service | all | Modal with warning about cascade check, "Delete" button |
| System | all | Modal with **red highlighted system name**, input field for user to type name |

> **UI Example (Production VM)**:
> ```
> ┌────────────────────────────────────────────────────┐
> │  ⚠️ Delete Production VM                          │
> │                                                    │
> │  You are about to delete:                         │
> │  [🔴 prod-shop-redis-01 ]                         │
> │                                                    │
> │  Type the VM name to confirm:                     │
> │  ┌──────────────────────────────────────────────┐ │
> │  │                                              │ │
> │  └──────────────────────────────────────────────┘ │
> │                                                    │
> │  [ Cancel ]                    [ Delete ]         │
> └────────────────────────────────────────────────────┘
> ```

> **Rationale for Name Confirmation**:
> - Forces user to consciously identify what they are deleting
> - Prevents "click-through" confirmation fatigue
> - Follows industry standard (GitHub, AWS, Kubernetes Dashboard)
> - Simple implementation, no server-side state required

---

### 14. Platform RBAC Model

**Decision**: Permissions are managed via **Platform RBAC tables** (Role, Permission, RoleBinding), not stored in entity fields. This enables OIDC/LDAP integration, fine-grained permissions, and environment-level isolation.

> **Design Principle**: "Platform RBAC, not Kubernetes RBAC". Shepherd maintains its own RBAC in PostgreSQL to provide multi-cluster unified access control, approval workflows, and business-level abstractions that K8s RBAC cannot support.

**Permission Inheritance Hierarchy**:

```
RoleBinding (user_id, role_id, scope_type, scope_id, allowed_environments)
   └── scope_type = "system" → applies to System + all child Services/VMs
   └── scope_type = "global" → applies to all resources (Admin only)
```

**Core RBAC Tables** (see [§22](#22-authentication--rbac-strategy) for full schema):

| Table | Purpose |
|-------|---------|
| `permissions` | Atomic permission definitions (e.g., `vm:create`, `system:read`) |
| `roles` | Permission bundles (e.g., `SystemAdmin`, `Operator`, `Viewer`) |
| `role_permissions` | Many-to-many: which permissions belong to which role |
| `role_bindings` | User-role assignments with scope and environment restrictions |

**Role-Based Visibility**:

| Role | Visibility Scope | Environment Access |
|------|------------------|-------------------|
| **PlatformAdmin** | All resources globally | test + prod |
| **SystemAdmin** | Assigned Systems + children | As specified in RoleBinding |
| **Operator** | Assigned Systems + children | As specified in RoleBinding |
| **Viewer** | Assigned Systems (read-only) | As specified in RoleBinding |

**Permission Matrix**:

| Operation | Permission Required | PlatformAdmin | SystemAdmin | Operator | Viewer |
|-----------|---------------------|---------------|-------------|----------|--------|
| View System/Service/VM | `system:read`, `service:read`, `vm:read` | ✅ | ✅ | ✅ | ✅ |
| Modify System description | `system:write` | ✅ | ✅ | ❌ | ❌ |
| Manage RoleBindings | `rbac:manage` | ✅ | ✅ | ❌ | ❌ |
| Create Service | `service:create` | ✅ | ✅ | ✅ | ❌ |
| Submit VM requests | `vm:create` | ✅ | ✅ | ✅ | ❌ |
| Operate VM (start/stop) | `vm:operate` | ✅ | ✅ | ✅ | ❌ |
| Access VNC | `vnc:access` | ✅ | ✅ | ✅ | ❌ |
| Approve requests | `approval:approve` | ✅ | ❌ | ❌ | ❌ |
| Manage clusters | `cluster:manage` | ✅ | ❌ | ❌ | ❌ |
| Manage templates | `template:manage` | ✅ | ❌ | ❌ | ❌ |

**Environment-Level Permission Control**:

```go
// RoleBinding includes environment restrictions
type RoleBinding struct {
    ID                  string    `json:"id"`
    UserID              string    `json:"user_id"`
    RoleID              string    `json:"role_id"`
    ScopeType           string    `json:"scope_type"`  // "global" | "system"
    ScopeID             *string   `json:"scope_id"`    // system_id if scope_type=system
    AllowedEnvironments []string  `json:"allowed_environments"` // ["test"] or ["test", "prod"]
    CreatedAt           time.Time `json:"created_at"`
    CreatedBy           string    `json:"created_by"`
}

// Example: Developer can only access test environment
// {user_id: "alice", role_id: "operator", scope_type: "system", scope_id: "sys-shop", allowed_environments: ["test"]}

// Example: SRE can access both test and prod
// {user_id: "bob", role_id: "operator", scope_type: "system", scope_id: "sys-shop", allowed_environments: ["test", "prod"]}
```

**Permission Resolution Implementation**:

```go
// Check permission for any entity, considering environment restrictions
func (s *PermissionService) HasPermission(ctx context.Context, 
    userID string, permission string, resourceType string, resourceID string, environment string) (bool, error) {
    
    // 1. Resolve resource to System
    systemID := s.resolveToSystemID(ctx, resourceType, resourceID)
    
    // 2. Query user's RoleBindings for this System (or global)
    bindings, _ := s.roleBindingRepo.Query(ctx).Where(
        rolebinding.UserIDEQ(userID),
        predicate.Or(
            rolebinding.ScopeTypeEQ("global"),
            predicate.And(
                rolebinding.ScopeTypeEQ("system"),
                rolebinding.ScopeIDEQ(systemID),
            ),
        ),
    ).WithRole(func(q *ent.RoleQuery) {
        q.WithPermissions()
    }).All(ctx)
    
    // 3. Check if any binding grants the required permission + environment
    for _, binding := range bindings {
        // Check environment restriction
        if !contains(binding.AllowedEnvironments, environment) {
            continue
        }
        
        // Check if role has required permission
        for _, perm := range binding.Edges.Role.Edges.Permissions {
            if perm.ID == permission || matchWildcard(perm.ID, permission) {
                return true, nil
            }
        }
    }
    
    return false, nil
}
```

**API Behavior**:

| Endpoint | PlatformAdmin | Non-Admin |
|----------|---------------|-----------|
| `GET /api/v1/systems` | Returns all systems | Returns systems where user has RoleBinding |
| `GET /api/v1/services` | Returns all services | Returns services under accessible systems |
| `GET /api/v1/vms` | Returns all VMs | Returns VMs under accessible systems, filtered by allowed_environments |
| `GET /api/v1/vms?environment=prod` | Returns prod VMs | Only if user's RoleBinding includes "prod" in allowed_environments |

**Rationale**: 
- **Platform RBAC > K8s RBAC**: Multi-cluster unified view, approval workflows, and business abstractions
- **Environment isolation**: Same role can have different environment access per binding
- **OIDC/LDAP ready**: RoleBindings can be auto-created from IdP group mappings
- **Audit-friendly**: All permission changes are tracked in RoleBinding table
- **No entity pollution**: Permissions are not stored in System/Service/VM entities

---

### 15. Cluster Visibility and Scheduling Strategy

**Decision**: End users do not see cluster information directly. Scheduling is based on namespace environment type matching cluster environment type.

**User Visibility**:

| Information | Regular User | Admin |
|-------------|--------------|-------|
| Cluster list | ❌ Hidden | ✅ Full access |
| Cluster in VM details | ✅ Read-only (after creation) | ✅ Full access |
| Namespace list | ✅ Filtered by environment permission | ✅ Full access |

**Environment-Based Scheduling**:

```
User with test permission → sees test namespaces → VMs scheduled to test clusters
User with prod permission → sees test+prod namespaces → VMs scheduled to matching cluster type
```

**Scheduling Strategy**:

| Phase | Actor | Logic |
|-------|-------|-------|
| Request | User | Selects namespace (environment implicitly determined) |
| Approval | Admin | System suggests cluster by weight; admin can override |
| Execution | Platform | Deploys to admin-selected or weight-based cluster |

**Cluster Weight Configuration**:

```go
// ent/schema/cluster.go
field.Int("scheduling_weight").Default(100).
    Comment("Higher weight = more likely to be selected for scheduling"),
field.Enum("environment").Values("test", "prod"),
```

**Weight-Based Selection**:

```go
func SelectCluster(clusters []Cluster, environment string) *Cluster {
    filtered := filterByEnvironment(clusters, environment)
    return weightedRandomSelect(filtered)
}
```

> **Note**: This platform is a governance layer. It does not interfere with Kubernetes-level resource scheduling (node selection, resource allocation, etc.).

---

### 16. Global Unique Naming and VM Name Format

**Decision**: System and Service names are globally unique. VM names include namespace prefix to ensure cluster-wide uniqueness. **Strict length constraints are enforced to guarantee Kubernetes DNS Label compatibility.**

> **Design Principle**: Prevent problems early. Warn users at entity creation time, not at VM creation time.

#### 16.1 Naming Length Constraints (Kubernetes DNS Label Standard)

> **Note**: While RFC 1123 allows labels to start with digits, **Kubernetes implements stricter requirements** that align with RFC 1035 for most resource types. This platform follows Kubernetes conventions to ensure maximum compatibility.

**Constraint Derivation**:

```
VM Name Format: {namespace}-{system}-{service}-{index}
                     │          │        │        │
                     │          │        │        └── 2 chars (00-99)
                     │          │        └────────── max 15 chars
                     │          └─────────────────── max 15 chars
                     └────────────────────────────── max 15 chars
                                                     ─────────────
                                    Separators: 3 chars (hyphens)
                                    Total max: 15+1+15+1+15+1+2 = 50 chars
                                    RFC 1123 limit: 63 chars ✅ Safe
```

**Entity Length Limits**:

| Entity | Max Length | Enforced At | Validation |
|--------|------------|-------------|------------|
| **Namespace** | 15 characters | Platform does not create namespaces; validated at VM request | Warn + Block |
| **System** | 15 characters | System creation API | Warn (soft) + Block (hard) |
| **Service** | 15 characters | Service creation API | Warn (soft) + Block (hard) |
| **VM Name** | 50 characters | VM creation (platform-generated) | Auto-calculated, always safe |

**Naming Character Rules** (Kubernetes DNS Label Standard):

> **Technical Note**: While RFC 1123 technically permits labels to start with digits, Kubernetes enforces stricter requirements aligned with RFC 1035 for most resource types (including VirtualMachine). This platform follows Kubernetes conventions to ensure maximum compatibility.

| Rule | Requirement | Rationale |
|------|-------------|----------|
| Characters | Lowercase alphanumeric (`a-z`, `0-9`) and hyphen (`-`) only | Kubernetes enforces lowercase |
| **Start Character** | Must start with an **alphabetic** character (`a-z`) | Kubernetes implementation choice (RFC 1035 aligned, not RFC 1123 literal) |
| End Character | Must end with alphanumeric character (`a-z`, `0-9`) | RFC 1123 requirement |
| No consecutive hyphens | Avoid `--` in names | Improves readability |
| No underscores | Use hyphens instead | DNS compatibility |

> **Technical Note**: Kubernetes `Service` objects can start with a digit if the `RelaxedServiceNameValidation` feature gate is enabled. However, this platform enforces stricter RFC 1035 rules for all entity names to ensure maximum compatibility across all Kubernetes resource types and versions.

#### 16.2 Early Warning Strategy (Shift-Left Validation)

> **Rationale**: Users should not experience "surprise failure" at VM creation time. Problems should be surfaced as early as possible.

**Three-Tier Validation**:

| Tier | Validation Point | Behavior |
|------|-----------------|----------|
| **Tier 1: Soft Warning** | Entity creation (System/Service) | API returns `warning` field; creation succeeds |
| **Tier 2: Hard Block** | Entity creation (exceeds limit) | API returns `400 Bad Request`; creation blocked |
| **Tier 3: Final Guard** | VM creation (impossible edge case) | API returns `400 Bad Request` with detailed explanation |

**Soft Warning Threshold** (Tier 1):

| Entity | Warn at | Block at |
|--------|---------|----------|
| System | > 12 chars | > 15 chars |
| Service | > 12 chars | > 15 chars |
| Namespace | > 12 chars | > 15 chars |

**API Warning Response (Tier 1)**:

```go
// System/Service creation with long name (but within limit)
// POST /api/v1/systems
// Request: {"name": "myverylongsystem", ...}  // 16 chars - BLOCKED
// Request: {"name": "mysystem1234", ...}       // 12 chars - WARN

type CreateEntityResponse struct {
    ID       string   `json:"id"`
    Name     string   `json:"name"`
    Warnings []string `json:"warnings,omitempty"` // Non-empty if soft warning triggered
}

// Example response with warning:
{
    "id": "sys-abc123",
    "name": "mysystem1234",
    "warnings": [
        "NAME_LENGTH_WARNING: System name is 12 characters. We recommend keeping names under 12 characters to ensure VM names remain readable. Current name will generate VM names like 'dev-mysystem1234-redis-01' (31+ chars)."
    ]
}
```

**Hard Block Response (Tier 2)**:

```go
// POST /api/v1/systems
// Request: {"name": "myverylongsystemname", ...}  // 20 chars - BLOCKED

// Response 400 Bad Request:
{
    "code": "NAME_TOO_LONG",
    "message": "System name exceeds maximum length",
    "params": {
        "entity": "system",
        "name": "myverylongsystemname",
        "length": 20,
        "max_length": 15,
        "suggestion": "Please use a name with 15 or fewer characters. Recommended: 12 or fewer for optimal readability."
    }
}
```

**Final Guard Response (Tier 3 - VM Creation)**:

```go
// This should rarely happen if Tier 1 and 2 work correctly
// But serves as a safety net for edge cases (e.g., namespace from external system)

// POST /api/v1/vms
// Response 400 Bad Request:
{
    "code": "VM_NAME_TOO_LONG",
    "message": "Generated VM name would exceed Kubernetes limit",
    "params": {
        "generated_name": "verylongnamespace-mybigsystem-myservice-01",
        "length": 45,
        "max_length": 50,
        "components": {
            "namespace": {"value": "verylongnamespace", "length": 17, "max": 15, "exceeds": true},
            "system": {"value": "mybigsystem", "length": 11, "max": 15, "exceeds": false},
            "service": {"value": "myservice", "length": 9, "max": 15, "exceeds": false}
        },
        "suggestion": "The namespace 'verylongnamespace' exceeds the 15-character limit. Please use a shorter namespace or contact your administrator."
    }
}
```

#### 16.3 Naming Rules Summary

| Entity | Uniqueness Scope | Format | Max Length |
|--------|------------------|--------|------------|
| System | Global | `{system_name}` | 15 chars |
| Service | Global | `{service_name}` | 15 chars |
| VM | Per Namespace (K8s) | `{namespace}-{system}-{service}-{index}` | 50 chars (derived) |

**VM Name Generation**:

```go
const (
    MaxNamespaceLength = 15
    MaxSystemNameLength = 15
    MaxServiceNameLength = 15
    MaxVMNameLength = 50  // 15+1+15+1+15+1+2 = derived, always safe
)

func GenerateVMName(namespace, systemName, serviceName string, index int) (string, error) {
    // Final safety check (should never trigger if earlier validations work)
    if len(namespace) > MaxNamespaceLength {
        return "", &ErrNameTooLong{Entity: "namespace", Name: namespace, Max: MaxNamespaceLength}
    }
    if len(systemName) > MaxSystemNameLength {
        return "", &ErrNameTooLong{Entity: "system", Name: systemName, Max: MaxSystemNameLength}
    }
    if len(serviceName) > MaxServiceNameLength {
        return "", &ErrNameTooLong{Entity: "service", Name: serviceName, Max: MaxServiceNameLength}
    }
    
    name := fmt.Sprintf("%s-%s-%s-%02d", namespace, systemName, serviceName, index)
    return name, nil
}

// Example: dev-shop-redis-01 (18 chars - well within 50 limit)
```

#### 16.4 FQDN (Fully Qualified Domain Name) Strategy

For internal DNS resolution, the recommended FQDN pattern:

```
{vm-name}.{namespace}.svc.cluster.local
```

Example:
```
dev-shop-redis-01.development.svc.cluster.local
```

**Platform Hostname Label**:

```yaml
labels:
  kubevirt-shepherd.io/hostname: dev-shop-redis-01
annotations:
  kubevirt-shepherd.io/fqdn: dev-shop-redis-01.development.svc.cluster.local
```

> **Best Practice**: If cross-cluster DNS is required, consider implementing a central DNS service (e.g., external-dns with multi-cluster support) as a future RFC.
>
> **Community Input**: The 15-character entity name limit is a V1 hard constraint to ensure Kubernetes compatibility. If your use case requires longer names, please [open a GitHub Issue](https://github.com/kv-shepherd/shepherd/issues) to discuss potential solutions for future versions.

#### 16.5 Implementation Checklist

| Check | Layer | Enforcement |
|-------|-------|-------------|
| System name ≤ 15 chars | API validation | Block creation |
| System name > 12 chars | API validation | Warn in response |
| Service name ≤ 15 chars | API validation | Block creation |
| Service name > 12 chars | API validation | Warn in response |
| Namespace ≤ 15 chars | VM request validation | Block with clear explanation |
| RFC 1123 character set | All entity creation | Block with suggestion |
| No consecutive hyphens | All entity creation | Block with suggestion |

---

### 17. Template Version Locking and Snapshot

**Decision**: VM creation uses the template version selected by admin at approval time. Template updates do not affect existing VMs. Template changes are tracked via snapshots.

**Template Usage Flow**:

```
1. User selects template (sees active version)
2. Request enters pending approval
3. Admin approves (may select different template/version)
4. Final template content is snapshotted to ApprovalTicket
5. VM created using snapshotted template
```

**Template Snapshot in ApprovalTicket**:

```go
// ApprovalTicket additional fields
field.Int("template_version").
    Comment("Template version at approval time"),
field.Text("template_snapshot").
    Comment("Full template content snapshot for audit"),
```

**VM Revision History** (existing design, reinforced):

```go
// ent/schema/vm_revision.go
field.Int("revision").
    Comment("Revision number, auto-incremented"),
field.Text("spec_snapshot").
    Comment("Full VM spec at this revision"),
field.String("change_reason").
    Comment("Reason for this change"),
field.Time("created_at"),
```

**Rationale**:
- Ensures reproducibility: can recreate exact VM from any point in history
- Facilitates debugging: compare template versions to identify issues
- Audit compliance: complete change history preserved

---

### 18. VNC Console Access Permissions

> **V1 Priority**: Low. VNC is a convenience feature for administrators, not a core governance function. We recommend enterprises use bastion hosts for production VM management. Security enhancements described here are targets for future versions.

**Decision**: VNC access requires approval in production environment, no approval needed in test environment.

**Permission Matrix**:

| Environment | VNC Access | Approval Required |
|-------------|------------|-------------------|
| test | ✅ Allowed | ❌ No |
| prod | ✅ Allowed | ✅ Yes (temporary grant) |

**Production VNC Flow**:

```
1. User requests VNC access to prod VM
2. Request creates approval ticket (VNC_ACCESS_REQUESTED)
3. Admin approves with time limit (e.g., 2 hours)
4. User gets temporary VNC token
5. Token expires after time limit
6. All VNC sessions are audit logged
```

**VNC Token Security Specification**:

| Security Measure | Implementation | Rationale |
|-----------------|----------------|----------|
| **Token Encryption** | AES-256-GCM encryption at rest | Protect stored tokens |
| **Single Use** | Token invalidated after first connection | Prevent replay attacks |
| **Time-Bounded** | Max TTL: 2 hours (configurable) | Limit exposure window |
| **User Binding** | Token includes hashed user ID | Prevent token sharing |
| **Revocation** | Admin can revoke active tokens | Emergency access termination |

**Encryption Key Management**:

> VNC token encryption shares the same key management infrastructure as cluster credential encryption (see [Phase 1: Multi-Cluster Credential Management](../design/phases/01-contracts.md#5-multi-cluster-credential-management)).

| Aspect | Specification |
|--------|---------------|
| Key Storage | Application-level secret (environment variable or external secret manager) |
| Key Rotation | Supported via `encryption_key_id` field; old tokens remain valid until expiry |
| Algorithm | AES-256-GCM (AEAD providing confidentiality and integrity) |
| Key Derivation | Per-token nonce generated via CSPRNG |

**Token Structure**:

```go
type VNCAccessToken struct {
    TokenID      string    `json:"token_id"`
    VMID         string    `json:"vm_id"`
    UserID       string    `json:"user_id"`
    TicketID     string    `json:"ticket_id"`      // Approval ticket reference
    ExpiresAt    time.Time `json:"expires_at"`
    UsedAt       *time.Time `json:"used_at,omitempty"`  // nil = not yet used
    RevokedAt    *time.Time `json:"revoked_at,omitempty"`
    CreatedAt    time.Time `json:"created_at"`
}

// Token is valid only if:
// 1. Not expired: time.Now() < ExpiresAt
// 2. Not used: UsedAt == nil
// 3. Not revoked: RevokedAt == nil
func (t *VNCAccessToken) IsValid() bool {
    return time.Now().Before(t.ExpiresAt) && t.UsedAt == nil && t.RevokedAt == nil
}
```

**Audit Logging**:

| Environment | Operation | Audit Logged |
|-------------|-----------|--------------|
| test | start/stop/restart | ✅ V1 (may relax in future) |
| test | VNC connect | ✅ V1 (may relax in future) |
| prod | All operations | ✅ Always required |
| prod | VNC token issued | ✅ Always (includes approver) |
| prod | VNC token used | ✅ Always (includes connection time) |
| prod | VNC token revoked | ✅ Always (includes revoker) |

> **Future Consideration**: Test environment audit logging for routine operations (start/stop/VNC) may be made configurable in later versions.
> 
> **V2 Consideration**: Session recording for production VMs may be added via RFC.

---

### 19. Batch Operations

**Decision**: Support batch operations with **parent-child ticket model** and **independent execution per item**.

> **Design Principle**: Batch operations are for user convenience, not for atomic guarantees. Each item executes independently, failures are isolated.

**Supported Batch Operations**:

| Operation | Max Batch Size | Ticket Model | Execution Mode |
|-----------|----------------|--------------|----------------|
| Batch Create VM | 10 | Parent + Child tickets | Independent per VM |
| Batch Start/Stop | 50 | Per environment policy | Best-effort |
| Batch Delete | 10 | Parent + Child tickets | Independent per VM |
| Batch Approve (Admin) | 20 | Admin action | Independent per ticket |

**Two-Layer Rate Limiting Strategy**:

> **Design Principle**: Protect system stability first, ensure user fairness second. Administrators retain full control.

**Layer 1: Global Rate Limiting (System Protection)**

| Limit | Value | Rationale |
|-------|-------|-----------|
| **Max global pending batch tickets** | 100 | Prevent system overload |
| **Max global API requests** | 1000 req/min | Infrastructure protection |

**Layer 2: User-Level Auto Rate Limiting (Fairness)**

> Automatically enforced to prevent "noisy neighbor" issues. Administrators can exempt users or adjust limits.

| Limit | Default Value | Rationale |
|-------|---------------|----------|
| **Max pending batch requests per user** | 3 | Prevent single user monopolizing queue |
| **Cooldown between batch submissions** | 2 minutes | Allow queue processing, reduce burst load |
| **Max total pending child tickets per user** | 30 | Limit resource commitment per user |

**Administrator Override Capabilities**:

| Operation | API Endpoint | Description |
|-----------|--------------|-------------|
| Exempt user from rate limiting | `POST /api/v1/admin/rate-limits/exemptions` | Grant user unlimited access |
| Remove user exemption | `DELETE /api/v1/admin/rate-limits/exemptions/{user_id}` | Restore normal rate limiting |
| Adjust user limits | `PUT /api/v1/admin/rate-limits/users/{user_id}` | Custom limits for specific user |
| View rate-limited users | `GET /api/v1/admin/rate-limits/status` | Monitor current rate limit status |

**Rate Limit Response**:

```go
// When user hits rate limit
type RateLimitExceededError struct {
    Code          string    `json:"code"`           // "RATE_LIMIT_EXCEEDED"
    LimitType     string    `json:"limit_type"`     // "global" or "user"
    CurrentValue  int       `json:"current_value"`
    MaxValue      int       `json:"max_value"`
    RetryAfter    int       `json:"retry_after"`    // Seconds until limit resets
    ContactAdmin  bool      `json:"contact_admin"`  // true if user should request exemption
}
```

> **Rationale**: Enterprise internal users are trusted. Auto rate limiting handles normal cases; administrators handle exceptions. No Redis required for V1 (counters stored in PostgreSQL).

**Atomicity Boundary (ADR-0012 Compliant)**:

> ⚠️ **Critical**: The creation phase and execution phase have different atomicity guarantees.

| Phase | Scope | Atomicity | Implementation |
|-------|-------|-----------|----------------|
| **Ticket Creation** | Parent + all Child tickets | ✅ Single atomic transaction | sqlc-only (ADR-0012) |
| **Ticket Execution** | Each Child ticket | ❌ Independent | River Worker per child |

**Ticket Creation Transaction**:

```go
// internal/usecase/batch_create_vm.go
// Uses sqlc-only transaction for ADR-0012 compliance

func (uc *BatchCreateVMUseCase) Execute(ctx context.Context, input BatchVMCreateInput) (*BatchVMCreateOutput, error) {
    tx, err := uc.pool.Begin(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx)
    
    sqlcTx := uc.queries.WithTx(tx)
    
    // 1. Create parent ticket
    parentTicketID := uuid.New().String()
    err = sqlcTx.CreateBatchApprovalTicket(ctx, db.CreateBatchApprovalTicketParams{
        TicketID:   parentTicketID,
        BatchType:  "BATCH_CREATE",
        ChildCount: input.Count,
        Status:     "PENDING_APPROVAL",
        Reason:     input.Reason,
    })
    if err != nil {
        return nil, fmt.Errorf("create parent ticket: %w", err)
    }
    
    // 2. Create all child tickets in same transaction
    for i := 0; i < input.Count; i++ {
        childTicketID := uuid.New().String()
        err = sqlcTx.CreateChildApprovalTicket(ctx, db.CreateChildApprovalTicketParams{
            TicketID:       childTicketID,
            ParentTicketID: parentTicketID,
            VMSpecJSON:     marshalVMSpec(input, i),
            Status:         "PENDING",
        })
        if err != nil {
            // Entire batch creation rolls back
            return nil, fmt.Errorf("create child ticket %d: %w", i, err)
        }
    }
    
    // 3. Atomic commit: all tickets created or none
    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("commit batch tickets: %w", err)
    }
    
    return &BatchVMCreateOutput{ParentTicketID: parentTicketID}, nil
}
```

> **Key Guarantee**: If any child ticket creation fails, the entire batch request is rejected. Users will never see a partially created batch.

**Parent-Child Ticket Model**:

```go
// Parent ticket: Batch request metadata
type BatchApprovalTicket struct {
    TicketID      string    `json:"ticket_id"`
    BatchType     string    `json:"batch_type"` // "BATCH_CREATE", "BATCH_DELETE"
    ChildCount    int       `json:"child_count"`
    SuccessCount  int       `json:"success_count"`
    FailedCount   int       `json:"failed_count"`
    PendingCount  int       `json:"pending_count"`
    Status        string    `json:"status"` // PENDING_APPROVAL, IN_PROGRESS, COMPLETED, PARTIAL_SUCCESS, FAILED
    Reason        string    `json:"reason"`
    CreatedAt     time.Time `json:"created_at"`
}

// Child ticket: Individual VM operation
type ChildApprovalTicket struct {
    TicketID       string `json:"ticket_id"`
    ParentTicketID string `json:"parent_ticket_id"`
    VMSpec         VMSpec `json:"vm_spec"`
    Status         string `json:"status"` // PENDING, APPROVED, COMPLETED, FAILED
    ErrorMessage   string `json:"error_message,omitempty"`
}
```

**Batch Request Structure**:

```go
type BatchVMCreateRequest struct {
    ServiceID  string   `json:"service_id" binding:"required"`
    TemplateID string   `json:"template_id" binding:"required"`
    Namespace  string   `json:"namespace" binding:"required"`
    Count      int      `json:"count" binding:"required,min=1,max=10"`
    Reason     string   `json:"reason" binding:"required"`
    
    // Optional per-VM overrides
    Instances []VMInstanceOverride `json:"instances,omitempty"`
}
```

**Execution Strategy**:

| Strategy | Description | Use Case |
|----------|-------------|----------|
| **Independent** | Each VM creates/deletes independently; failures don't affect others | Batch create, batch delete |
| **Best-effort** | Execute all, record partial success | Power operations |

**Parent Ticket Status Calculation**:

```go
func (t *BatchApprovalTicket) CalculateStatus() string {
    if t.PendingCount > 0 {
        return "IN_PROGRESS"
    }
    if t.FailedCount == 0 {
        return "COMPLETED"
    }
    if t.SuccessCount == 0 {
        return "FAILED"
    }
    return "PARTIAL_SUCCESS"
}
```

**Rationale for Independent Execution**:
- Cross-cluster operations cannot be atomic (no distributed transaction)
- Partial success is better than total rollback for user experience
- Each child ticket provides clear retry capability for failed items
- Aligns with Kubernetes Job's `backoffLimitPerIndex` pattern
- **Ticket creation atomicity** ensures users never see partial batch states

---

### 20. Notification System

**Decision**: V1 implements platform-internal inbox. Design is decoupled to allow future integration with external notification systems.

**V1 Implementation - Internal Inbox**:

```go
// ent/schema/notification.go
func (Notification) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("recipient").NotEmpty(),           // Username
        field.Enum("type").Values(
            "APPROVAL_REQUIRED",
            "REQUEST_APPROVED", 
            "REQUEST_REJECTED",
            "VM_CREATED",
            "VM_DELETED",
        ),
        field.String("title").NotEmpty(),
        field.Text("content"),
        field.String("related_ticket_id").Optional(),
        field.Bool("read").Default(false),
        field.Time("created_at").Default(time.Now),
        field.Time("read_at").Optional().Nillable(),
    }
}
```

**Decoupled Interface**:

```go
type NotificationSender interface {
    Send(ctx context.Context, notification *Notification) error
    SendBatch(ctx context.Context, notifications []*Notification) error
}

// V1: InboxNotificationSender (database)
// Future: EmailNotificationSender, WebhookNotificationSender, SlackNotificationSender
```

**Notification Triggers**:

| Event | Recipients | Channel (V1) |
|-------|------------|--------------|
| New approval request | All admins | Inbox |
| Request approved/rejected | Request creator + maintainers | Inbox |
| VM created/deleted | Request creator + maintainers | Inbox |
| Approval pending 7+ days | All admins | Inbox (highlighted) |

---

### 21. Scope Exclusions (V1)

The following features are explicitly out of scope for V1:

| Feature | Status | Notes |
|---------|--------|-------|
| Resource Quota | ❌ Not in V1 | May add in future RFC |
| User-defined Business Tags | ❌ Not in V1 | Will store in DB not K8s if added |
| Multi-tenancy (Full) | ❌ Not in V1 | Schema reserved, full isolation deferred |
| Complex Approval Workflows | ❌ Not in V1 | See RFC-0002 for Temporal integration |
| Approval Timeout Auto-processing | ❌ Not in V1 | UI prioritization used instead |

**Multi-tenancy Clarification**:

V1 reserves the `tenant_id` field in schema but does not implement full multi-tenancy features.

| Aspect | V1 Behavior | Future Multi-tenancy |
|--------|-------------|----------------------|
| `tenant_id` field | ✅ Exists in schema | ✅ Required |
| Value in V1 | Fixed: `"default"` | Unique per tenant |
| Query filter | Not applied | Auto-applied for isolation |
| Data isolation | ❌ Not enforced | ✅ Strict isolation |
| Tenant admin role | ❌ Not available | ✅ Per-tenant admin |
| Separate billing | ❌ Not available | ✅ Per-tenant billing |

**Future Tenant Scenario Definition**:

> **Scope**: Tenants represent **departments within the same enterprise**, NOT separate companies.
>
> | Scenario | In Scope | Out of Scope |
> |----------|----------|--------------|
> | Enterprise departments (HR, IT, Finance) | ✅ | - |
> | Business units with budget separation | ✅ | - |
> | Multi-company SaaS platform | - | ❌ |
> | External customer isolation | - | ❌ |

**Rationale for Reservation**:
- Schema stability: Adding `tenant_id` later requires data migration
- Low cost: A constant value has minimal runtime overhead
- Future-ready: When department isolation is needed, only business logic changes required

**Implementation in V1**:

```go
const DefaultTenantID = "default"

// All entities include tenant_id with fixed value
field.String("tenant_id").Default(DefaultTenantID).Immutable()
```

---

### 22. Authentication & RBAC Strategy

**Decision**: Implement **Platform RBAC** with full database-backed permission management. Support OIDC/LDAP integration via **guided configuration flow** (sample data → field selection → group sync → visual mapping).

> **Strategic Position**: Shepherd maintains its own RBAC in PostgreSQL rather than relying on Kubernetes RBAC. This enables multi-cluster unified access control, approval workflows, and business-level abstractions that K8s RBAC cannot provide.

#### 22.1 Platform RBAC vs Kubernetes RBAC

| Aspect | K8s RBAC | Platform RBAC (Shepherd) |
|--------|----------|--------------------------|
| Scope | Single cluster | Multi-cluster unified |
| Abstraction | Namespace/Resource | System/Service/Environment |
| Approval Workflow | ❌ Not supported | ✅ Native support |
| OIDC/LDAP Integration | Complex (per-cluster) | ✅ Centralized |
| Audit Trail | Separate per cluster | ✅ Unified in PostgreSQL |
| Business Logic | ❌ Not possible | ✅ Environment-based policies |

**Rationale**: Kubernetes RBAC is designed for cluster-level resource access, not for business-level governance. Shepherd provides a governance layer that abstracts away K8s complexity while maintaining security.

**Multi-Cluster Stability Architecture**:

> This platform maintains RBAC in PostgreSQL rather than extending Kubernetes-native RBAC stored in etcd. This architectural choice aligns with the platform's positioning as a **governance layer** optimized for multi-tenant, multi-user scenarios.

| Consideration | K8s-native RBAC (etcd) | Platform RBAC (PostgreSQL) |
|---------------|------------------------|---------------------------|
| **Query Load Impact** | Permission checks traverse K8s API Server → etcd | Handled entirely by PostgreSQL, K8s control plane unaffected |
| **Multi-tenant Scalability** | etcd optimized for cluster state, not high-frequency user queries | PostgreSQL handles relational queries with indexing and connection pooling |
| **Rate Limiting** | Requires K8s API Priority & Fairness configuration | Native application-level control, independent of K8s |
| **Cluster Stability** | User permission queries share etcd resources with critical cluster operations | Zero contention with K8s API Server or etcd |
| **Query Complexity** | Limited to Kubernetes RBAC model | Flexible SQL queries supporting environment-based, time-based policies |

> By externalizing RBAC to PostgreSQL, Shepherd ensures that user permission checks, role lookups, and multi-tenant access patterns do not contend with critical Kubernetes operations (scheduling, pod lifecycle, etc.), thereby preserving cluster stability under high user concurrency.

#### 22.2 RBAC Core Schema

```go
// ent/schema/permission.go
// Atomic permission definitions
func (Permission) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),      // "vm:create", "system:read"
        field.String("name").NotEmpty(),               // "Create VM"
        field.String("description").Optional(),        // "Allows creating new VMs"
        field.String("resource").NotEmpty(),           // "vm", "system", "cluster"
        field.String("action").NotEmpty(),             // "create", "read", "write", "delete"
    }
}

// ent/schema/role.go
// Role = bundle of permissions
func (Role) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),       // "role-systemadmin"
        field.String("name").NotEmpty(),                // "SystemAdmin"
        field.String("description").Optional(),
        field.Bool("is_builtin").Default(false),        // true = cannot be deleted
        field.Time("created_at").Default(time.Now),
    }
}

func (Role) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("permissions", Permission.Type),  // Many-to-many
    }
}

// ent/schema/role_binding.go
// Assigns role to user with scope and environment restrictions
func (RoleBinding) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("user_id").NotEmpty(),
        field.String("role_id").NotEmpty(),
        field.Enum("scope_type").Values("global", "system"),
        field.String("scope_id").Optional().Nillable(), // system_id if scope_type=system
        field.Strings("allowed_environments").Default([]string{"test"}),  // ["test"] or ["test", "prod"]
        field.Time("created_at").Default(time.Now),
        field.String("created_by").NotEmpty(),           // Who granted this binding
    }
}

func (RoleBinding) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("role", Role.Type).Unique().Required(),
    }
}

func (RoleBinding) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("user_id", "role_id", "scope_type", "scope_id").Unique(),
    }
}
```

#### 22.3 Built-in Roles and Permissions

**Permissions (Seeded at initialization)**:

| Permission ID | Resource | Action | Description |
|--------------|----------|--------|-------------|
| `system:read` | system | read | View system details |
| `system:write` | system | write | Modify system description |
| `system:delete` | system | delete | Delete system |
| `service:read` | service | read | View service details |
| `service:create` | service | create | Create new service |
| `service:delete` | service | delete | Delete service |
| `vm:read` | vm | read | View VM details |
| `vm:create` | vm | create | Submit VM creation request |
| `vm:operate` | vm | operate | Start/stop/restart VM |
| `vm:delete` | vm | delete | Delete VM |
| `vnc:access` | vnc | access | Access VNC console |
| `approval:approve` | approval | approve | Approve/reject requests |
| `approval:view` | approval | view | View pending approvals |
| `cluster:manage` | cluster | manage | Manage cluster configurations |
| `template:manage` | template | manage | Manage VM templates |
| `rbac:manage` | rbac | manage | Manage role bindings |
| `*:*` | * | * | Wildcard: all permissions |

**Built-in Roles (Seeded at initialization, `is_builtin=true`)**:

| Role ID | Name | Permissions |
|---------|------|-------------|
| `role-platform-admin` | PlatformAdmin | `*:*` (all) |
| `role-system-admin` | SystemAdmin | `system:*`, `service:*`, `vm:*`, `vnc:access`, `rbac:manage` |
| `role-operator` | Operator | `system:read`, `service:read`, `vm:*`, `vnc:access` |
| `role-viewer` | Viewer | `system:read`, `service:read`, `vm:read` |

```go
// Seed data at application startup
func SeedBuiltinRoles(ctx context.Context, db *ent.Client) error {
    // Permissions
    permissions := []Permission{
        {ID: "system:read", Resource: "system", Action: "read", Name: "View System"},
        {ID: "system:write", Resource: "system", Action: "write", Name: "Edit System"},
        // ... more permissions
        {ID: "*:*", Resource: "*", Action: "*", Name: "All Permissions"},
    }
    
    // Roles with permission assignments
    roles := []struct {
        Role        Role
        Permissions []string
    }{
        {
            Role: Role{ID: "role-platform-admin", Name: "PlatformAdmin", IsBuiltin: true},
            Permissions: []string{"*:*"},
        },
        {
            Role: Role{ID: "role-system-admin", Name: "SystemAdmin", IsBuiltin: true},
            Permissions: []string{"system:*", "service:*", "vm:*", "vnc:access", "rbac:manage"},
        },
        // ... more roles
    }
    
    // Upsert logic...
    return nil
}
```

#### 22.4 IdP Integration: Guided Configuration Flow

> **Design Principle**: "Show, don't ask". Display sample data to admins, let them choose fields visually, rather than requiring manual configuration.

**Step 1: Connect & Sample**

```go
// Admin initiates IdP connection, system fetches 10 sample users
type IdpSampleResponse struct {
    Users       []map[string]interface{} `json:"users"`        // 10 sample tokens
    DetectedFields []FieldInfo           `json:"detected_fields"`
}

type FieldInfo struct {
    Name         string   `json:"name"`            // "groups", "department", "roles"
    Type         string   `json:"type"`            // "array" | "string"
    SampleValues []string `json:"sample_values"`   // ["DevOps-Team", "QA-Team"]
    UniqueCount  int      `json:"unique_count"`    // Number of unique values in sample
}

// API: GET /api/v1/admin/idp/{id}/sample
```

**Step 2: Admin Selects Field**

```
UI Display:
┌─────────────────────────────────────────────────────────┐
│  Sample Token Fields:                                    │
│  ┌───────────────────────────────────────────────────┐  │
│  │ ◉ groups (array, 5 unique values)                 │  │
│  │   Sample: ["DevOps-Team", "QA-Team", ...]         │  │
│  │ ○ department (string, 3 unique values)            │  │
│  │   Sample: ["Engineering", "IT", "QA"]             │  │
│  │ ○ custom_roles (array, 2 unique values)           │  │
│  │   Sample: ["admin", "developer"]                  │  │
│  └───────────────────────────────────────────────────┘  │
│                                                          │
│  [Sync Selected Field →]                                 │
└─────────────────────────────────────────────────────────┘
```

**Step 3: Sync Unique Values (Deduped)**

```go
// Sync only the unique values of selected field (not all users!)
type IdpGroupSyncRequest struct {
    IdpConfigID string `json:"idp_config_id"`
    FieldName   string `json:"field_name"`  // Admin-selected field
}

// API: POST /api/v1/admin/idp/{id}/sync-groups
// Result: Syncs unique group names to idp_synced_groups table

// ent/schema/idp_synced_group.go
func (IdpSyncedGroup) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("idp_config_id").NotEmpty(),
        field.String("group_id").NotEmpty(),       // The actual value from IdP
        field.String("group_name").Optional(),      // Human-readable name if available
        field.String("source_field").NotEmpty(),    // "groups", "department", etc.
        field.Time("synced_at").Default(time.Now),
    }
}
```

**Step 4: Visual Role Mapping**

```
UI Display:
┌─────────────────────────────────────────────────────────┐
│  IdP Group → Shepherd Role Mapping                       │
│  ┌───────────────────────────────────────────────────┐  │
│  │ IdP Group          Shepherd Role    Environments  │  │
│  │ ─────────────────────────────────────────────────│  │
│  │ DevOps-Team       [SystemAdmin ▼]   ☑test ☑prod  │  │
│  │ QA-Team           [Operator ▼]      ☑test ☐prod  │  │
│  │ IT-Support        [Viewer ▼]        ☑test ☐prod  │  │
│  │ HR-Department     [No Mapping ▼]    -            │  │
│  └───────────────────────────────────────────────────┘  │
│                                                          │
│  💡 Unmapped groups get default: Viewer + test only     │
│                                                          │
│  [💾 Save Mapping]                                       │
└─────────────────────────────────────────────────────────┘
```

**Mapping Storage**:

```go
// ent/schema/idp_group_mapping.go
func (IdpGroupMapping) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("idp_config_id").NotEmpty(),
        field.String("idp_group_id").NotEmpty(),          // Links to idp_synced_groups
        field.String("idp_group_name").Optional(),         // Cached for display
        field.String("role_id").NotEmpty(),                // Shepherd role
        field.Enum("scope_type").Values("global", "system").Default("global"),
        field.String("scope_id").Optional().Nillable(),
        field.Strings("allowed_environments").Default([]string{"test"}),
        field.Time("created_at").Default(time.Now),
    }
}
```

#### 22.5 Login Flow with IdP

```go
func HandleOIDCCallback(ctx context.Context, token *oidc.IDToken) (*User, error) {
    // 1. Extract claims
    var claims map[string]interface{}
    token.Claims(&claims)
    
    // 2. Get IdP config to know which field to use
    idpConfig := getIdpConfig(token.Issuer)
    groupField := idpConfig.ClaimsMapping.Groups  // e.g., "groups"
    
    // 3. Extract group values from token
    userGroups := extractField(claims, groupField)  // ["DevOps-Team", "QA-Team"]
    
    // 4. Query mappings
    mappings, _ := queryIdpGroupMappings(idpConfig.ID, userGroups)
    
    // 5. Create/Update RoleBindings for user
    for _, mapping := range mappings {
        upsertRoleBinding(ctx, RoleBinding{
            UserID:              token.Subject,
            RoleID:              mapping.RoleID,
            ScopeType:           mapping.ScopeType,
            ScopeID:             mapping.ScopeID,
            AllowedEnvironments: mapping.AllowedEnvironments,
            CreatedBy:           "idp-sync",
        })
    }
    
    // 6. Apply default role if no mappings found
    if len(mappings) == 0 {
        upsertRoleBinding(ctx, RoleBinding{
            UserID:              token.Subject,
            RoleID:              idpConfig.DefaultRoleID,  // "role-viewer"
            ScopeType:           "global",
            AllowedEnvironments: idpConfig.DefaultAllowedEnvironments,  // ["test"]
            CreatedBy:           "idp-default",
        })
    }
    
    return createOrUpdateUser(ctx, token)
}
```

#### 22.6 IdP Configuration Schema

```go
// ent/schema/idp_config.go
func (IdpConfig) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("name").NotEmpty(),                    // "Company Azure AD"
        field.Enum("type").Values("oidc", "ldap", "saml"),
        field.Bool("enabled").Default(true),
        
        // OIDC-specific config
        field.String("issuer").Optional(),
        field.String("client_id").Optional(),
        field.String("client_secret_encrypted").Optional(),
        
        // Claims mapping (guided by sample data)
        field.JSON("claims_mapping", ClaimsMapping{}),
        
        // Default permissions for unmapped users
        field.String("default_role_id").Default("role-viewer"),
        field.Strings("default_allowed_environments").Default([]string{"test"}),
        
        field.Time("created_at").Default(time.Now),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

type ClaimsMapping struct {
    UserID      string `json:"user_id"`       // Default: "sub"
    Email       string `json:"email"`         // Default: "email"
    DisplayName string `json:"display_name"`  // Default: "name"
    Groups      string `json:"groups"`        // Admin-selected field
    GroupsFormat string `json:"groups_format"` // "array" | "csv" | "ldap_dn"
}
```

**IdP Security Requirements (REQUIRED)**:

| Requirement | Implementation | Rationale |
|-------------|----------------|----------|
| **LDAP TLS** | LDAP connections MUST use `ldaps://` or StartTLS | Protect credentials in transit |
| **OIDC Token Validation** | Validate signature + `iss` + `aud` before trusting claims | Prevent token forgery |
| **Short-lived Access Tokens** | Access token TTL should be ≤ 1 hour | Limit exposure window |
| **Claims Refresh** | Group memberships re-evaluated on each login | Reflect IdP changes promptly |
| **Secret Encryption** | Uses same key management as cluster credentials (§5) | Unified encryption infrastructure |

```go
// Token validation example (REQUIRED checks)
func ValidateIDToken(token *oidc.IDToken, expectedIssuer, expectedAudience string) error {
    if token.Issuer != expectedIssuer {
        return ErrInvalidIssuer
    }
    if !contains(token.Audience, expectedAudience) {
        return ErrInvalidAudience
    }
    // Signature validation handled by oidc library
    return nil
}
```

#### 22.7 API Endpoints for RBAC

| Endpoint | Method | Purpose | Auth |
|----------|--------|---------|------|
| `/api/v1/admin/roles` | GET | List all roles | PlatformAdmin |
| `/api/v1/admin/roles` | POST | Create custom role | PlatformAdmin |
| `/api/v1/admin/roles/{id}/permissions` | PUT | Update role permissions | PlatformAdmin |
| `/api/v1/admin/role-bindings` | GET | List all role bindings | PlatformAdmin |
| `/api/v1/admin/role-bindings` | POST | Create role binding | PlatformAdmin or SystemAdmin (for own system) |
| `/api/v1/admin/role-bindings/{id}` | DELETE | Remove role binding | PlatformAdmin or SystemAdmin |
| `/api/v1/admin/idp` | GET | List IdP configurations | PlatformAdmin |
| `/api/v1/admin/idp` | POST | Create IdP configuration | PlatformAdmin |
| `/api/v1/admin/idp/{id}/sample` | GET | Get sample token data | PlatformAdmin |
| `/api/v1/admin/idp/{id}/sync-groups` | POST | Sync groups from IdP | PlatformAdmin |
| `/api/v1/admin/idp/{id}/mappings` | GET | List group mappings | PlatformAdmin |
| `/api/v1/admin/idp/{id}/mappings` | PUT | Update group mappings | PlatformAdmin |
| `/api/v1/me/permissions` | GET | Get current user's permissions | Authenticated |

---

## Consequences

### Positive

- ✅ **Scalability**: System decoupling enables multi-cluster, multi-namespace growth
- ✅ **Platform RBAC**: Database-backed RBAC enables OIDC/LDAP integration, fine-grained permissions, and environment-level isolation
- ✅ **Multi-Cluster Unified Access**: Single RBAC model manages permissions across all clusters
- ✅ **OIDC/LDAP Ready**: Guided configuration flow simplifies IdP integration without manual UUID mapping
- ✅ **Environment Isolation**: RoleBinding's `AllowedEnvironments` enforces test/prod separation per user
- ✅ **Data Integrity**: Immutable Service names and platform-controlled labels ensure traceability
- ✅ **Security**: User-forbidden fields (cloud_init, labels) prevent governance bypass
- ✅ **Flexibility**: Template masks enable feature rollout without code changes
- ✅ **Auditability**: Comprehensive event tracking for compliance requirements
- ✅ **Operational Safety**: Environment-based approval policies reduce production risks
- ✅ **Delete Protection**: Tiered confirmation mechanism prevents accidental deletions
- ✅ **Production Safety**: Name typing confirmation for production VM deletion follows industry standard (GitHub/AWS pattern)
- ✅ **Reduced Configuration**: Auto-detected storage classes simplify cluster onboarding
- ✅ **Request Protection**: Duplicate prevention avoids conflicting pending requests
- ✅ **Clear Naming**: Global unique naming and namespace-prefixed VM names prevent conflicts
- ✅ **RFC 1123 Compliance**: Strict naming constraints guarantee Kubernetes compatibility
- ✅ **Early Warning**: Shift-left validation warns users about long names at creation time
- ✅ **Version Safety**: Template snapshots ensure reproducibility and debugging capability
- ✅ **Batch Efficiency**: Parent-child ticket model provides clear progress tracking and retry capability
- ✅ **Batch Atomicity**: Ticket creation is atomic; users never see partially created batches
- ✅ **Fault Isolation**: Independent batch execution prevents cascade failures
- ✅ **Future-Proof Notifications**: Decoupled notification interface enables easy integration
- ✅ **VNC Security**: Single-use, time-bounded, user-bound tokens protect console access
- ✅ **Custom Roles**: Ability to create custom roles enables enterprise-specific permission bundles

### Negative

- 🟡 **Breaking Change**: Existing Phase 1 schema designs need updates
- 🟡 **Frontend Dependency**: Template mask, notification inbox, RBAC management UI, and batch UI require frontend work
- 🟡 **Hard Delete Risk**: Accidental deletion is permanent (mitigated by confirmation mechanism and cascade constraints)
- 🟡 **No Timeout**: Pending requests can accumulate without automatic cleanup
- 🟡 **RBAC Query Overhead**: Permission checks require RoleBinding + Role + Permission JOINs (PostgreSQL indexing sufficient for V1 scale; caching may be added via RFC if needed)
- 🟡 **Batch Partial Success**: Users must handle partial success states for batch operations
- 🟡 **Naming Constraints**: 15-character limit may require existing systems to rename
- 🟡 **IdP Dependency**: OIDC/LDAP integration requires external IdP availability

### Mitigation

- Phase 1 implementation has not started; schema updates can be incorporated directly
- Template mask structure is JSON-based; frontend can evolve independently
- Clear API contracts defined for frontend-backend coordination
- Tiered delete confirmation (confirm param + name typing) prevents accidental deletions
- Cascade constraints prevent deletion of entities with children
- UI-based prioritization and duplicate lock encourage timely processing
- RBAC queries optimized via PostgreSQL indexes on `user_id`, `role_id`, `scope_type`, `scope_id`; no application-level cache needed for V1
- Edge queries are optimized via Ent ORM's eager loading; overhead is minimal
- Rate limiting uses PostgreSQL counters, no external Redis dependency
- Batch status API provides clear visibility into partial success/failure states
- Naming constraints are clearly documented; soft warnings at 12 chars guide users proactively
- VNC tokens are auto-cleaned by periodic job; no manual maintenance required

---

## Implementation Impact

> **Note**: This section provides detailed change specifications for affected documents. These changes should be applied after ADR-0015 is accepted.

---

### Documents Requiring Updates (Detailed)

#### 0. `README.md` (Project Root)

| Section | Current State | Required Change |
|---------|---------------|-----------------|
| Design Principles | Contains 4 principles | Add **Platform RBAC** principle |

**Suggested Addition** (in Design Principles table):

| Principle | Description |
|-----------|-------------|
| **Platform RBAC** | RBAC in PostgreSQL, not Kubernetes. Permission queries isolated from cluster control plane. |

> **Rationale**: This is a differentiating architectural decision that users/contributors should understand upfront. It explains why Shepherd doesn't rely on K8s RBAC.

#### 1. `docs/design/phases/01-contracts.md`

| Section | Current State | Required Change |
|---------|---------------|-----------------|
| System Schema (§3.1) | Contains `namespace`, `environment` fields | **Remove** these fields, add `maintainers []string` |
| System Indexes | `index.Fields("namespace", "name").Unique()` | Change to `index.Fields("name").Unique()` (global unique) |
| Service Schema (§3.2) | Contains `created_by` field | **Remove** `created_by` and any `maintainers` field |
| VM Schema | May contain direct `system_id` | **Remove** direct `system_id`, keep only `service_id` edge |
| Governance Hierarchy (§1) | `Namespace → System → Service → VM` | Update to `System → Service → VM` (decoupled from namespace) |
| Labels (§2) | Basic label list | Add `hostname`, `ticket-id`, `created-by` labels |
| ApprovalTicket Schema | Basic fields | Add `template_version`, `template_snapshot`, `selected_storage_class` |
| **New**: BatchApprovalTicket | Not exists | Add parent-child ticket schema |
| **New**: ChildApprovalTicket | Not exists | Add child ticket schema with `parent_ticket_id` |

#### 2. `docs/design/phases/03-service-layer.md`

| Section | Current State | Required Change |
|---------|---------------|-----------------|
| Approval Matrix (§4) | Missing power operations | Add START_VM, STOP_VM, RESTART_VM with environment-based rules |
| Delete Pattern (§4) | Only Restrict described | Add tiered confirmation mechanism (confirm param / name typing) |
| **New**: Batch Operations | Not exists | Add batch operation processing patterns |
| **New**: Permission Resolution | Not exists | Add System-based permission inheritance pattern |
| Transaction Rules | Basic rules | Add batch ticket atomic creation pattern |

#### 3. `docs/design/phases/04-governance.md`

| Section | Current State | Required Change |
|---------|---------------|-----------------|
| Approval Types (§4) | Missing power ops | Add power operation types + VNC_ACCESS_REQUESTED |
| Environment Rules | Basic test/prod rules | Add detailed per-operation environment matrix |
| Status Flow | Basic flow | Add CANCELLED status + batch status (PARTIAL_SUCCESS) |
| **New**: Notification | Not exists | Add notification triggers and inbox API |
| **New**: Delete Confirmation | Not exists | Add tiered confirmation flow |
| **New**: VNC Permissions | Not exists | Add VNC access control by environment |

#### 4. `docs/design/examples/domain/vm.go`

| Field/Type | Current State | Required Change |
|------------|---------------|-----------------|
| `VM.SystemID` | Exists | **REMOVE COMPLETELY** - obtain via `Service.Edges.System`. Keeping this field risks inconsistency if developers mistakenly use it directly instead of querying through Service edge. |
| `VMSpec.SystemID` | Exists | **REMOVE COMPLETELY** - same rationale as above |
| `VMSpec.Name` | Exists | Add comment: "Platform-generated, user cannot set" |
| `VMSpec.Labels` | Exists | Add comment: "Platform-managed, user cannot set" |
| `VMSpec.CloudInit` | Exists | Add comment: "Template-defined only" |
| `VMCreateRequest` | May allow forbidden fields | Ensure `name`, `labels`, `cloud_init` are NOT in request struct |
| **New**: `BatchVMCreateRequest` | Not exists | Add batch request structure |
| **New**: `VMInstanceOverride` | Not exists | Add per-VM override structure |

#### 5. `docs/design/examples/domain/event.go`

| Event Type | Current State | Required Change |
|------------|---------------|-----------------|
| Power ops events | Not exists | Add `VM_START_REQUESTED`, `VM_STOP_REQUESTED`, `VM_RESTART_REQUESTED` |
| Cancellation events | Basic | Add detailed `REQUEST_CANCELLED` with reason payload |
| VNC events | Not exists | Add `VNC_ACCESS_REQUESTED`, `VNC_ACCESS_GRANTED` |
| Batch events | Not exists | Add `BATCH_CREATE_REQUESTED`, `BATCH_DELETE_REQUESTED` |
| Notification events | Not exists | Add `NOTIFICATION_SENT` |

#### 6. `docs/design/checklist/phase-1-checklist.md`

| Check Item | Current State | Required Change |
|------------|---------------|-----------------|
| System Schema | Checks `namespace` field | **Remove** namespace check, add `maintainers` check |
| Service Schema | Checks `created_by`, `maintainers` | **Remove** these checks (Service inherits from System) |
| VM Schema | Checks `system_id` | **Remove** - VM only has `service_id` edge |
| Template Schema | Basic | Add `quick_fields`, `advanced_fields`, `field_constraints` checks |
| **New**: Permission Inheritance | Not exists | Add check for Service/VM permission resolution |

#### 7. `docs/design/checklist/phase-4-checklist.md`

| Check Item | Current State | Required Change |
|------------|---------------|-----------------|
| Storage Class | Not exists | Add auto-detection and admin default checks |
| Notification | Not exists | Add internal inbox and interface checks |
| Batch Operations | Not exists | Add parent-child ticket and status checks |
| Delete Confirmation | Not exists | Add tiered confirmation mechanism checks |
| VNC Permissions | Not exists | Add environment-based VNC access checks |

---

### New API Endpoints

#### Added Endpoints

| Endpoint | Method | Purpose | Auth |
|----------|--------|---------|------|
| `/api/v1/approvals/{id}/cancel` | POST | User cancels pending request | Owner/Maintainer |
| `/api/v1/notifications` | GET | Get user notification inbox | Authenticated |
| `/api/v1/notifications/{id}/read` | POST | Mark notification as read | Owner |
| `/api/v1/notifications/read-all` | POST | Mark all as read | Owner |
| `/api/v1/admin/clusters/{id}/storage-classes` | GET | Get cluster storage classes | Admin |
| `/api/v1/admin/clusters/{id}/storage-classes/default` | PUT | Set default storage class | Admin |
| `/api/v1/vms/batch` | POST | Batch create VMs | Authenticated |
| `/api/v1/vms/batch/{id}` | GET | Get batch operation status | Owner/Maintainer |
| `/api/v1/vms/batch/{id}/retry` | POST | Retry failed items | Owner/Maintainer |
| `/api/v1/admin/rate-limits/exemptions` | POST | Exempt user from rate limiting | PlatformAdmin |
| `/api/v1/admin/rate-limits/exemptions/{user_id}` | DELETE | Remove user rate limit exemption | PlatformAdmin |
| `/api/v1/admin/rate-limits/users/{user_id}` | PUT | Adjust rate limits for specific user | PlatformAdmin |
| `/api/v1/admin/rate-limits/status` | GET | View current rate limit status | PlatformAdmin |

#### Modified Endpoints

| Endpoint | Change |
|----------|--------|
| `DELETE /api/v1/systems/{id}` | Requires `confirm_name` in request body matching system name |
| `DELETE /api/v1/services/{id}` | Requires `confirm=true` query parameter |
| `DELETE /api/v1/vms/{id}` | Requires `confirm=true` query parameter |
| `POST /api/v1/vms` | Rejects requests containing `name`, `cloud_init`, or `labels` fields |
| `GET /api/v1/services` | Filters by accessible Systems (not by Service-level permissions) |
| `GET /api/v1/vms` | Filters by accessible Systems (not by Service-level permissions) |

---

### New CI Checks Required

| Check Script | Validation Target | Trigger |
|--------------|-------------------|---------|
| `check_vm_name_format.go` | VM names follow `{ns}-{sys}-{svc}-{##}` | VM creation |
| `check_forbidden_user_fields.go` | Requests don't contain `name`/`cloud_init`/`labels` | API validation |
| `check_platform_labels.go` | Platform labels exist and immutable | K8s resource apply |
| `check_duplicate_request.go` | No duplicate pending requests | New request submission |
| `check_cascade_constraints.go` | No child resources before delete | Delete request |
| `check_delete_confirmation.go` | Confirmation parameter/body present | Delete API call |
| `check_notification_interface.go` | NotificationSender interface implemented | Build time |
| `check_permission_inheritance.go` | Service/VM permissions resolve to System | Permission check |
| `check_batch_ticket_consistency.go` | Parent-child ticket counts consistent | Batch operations |
| `check_name_length.go` | System/Service name ≤ 15 chars, warn > 12 chars | Entity creation |
| `check_k8s_dns_label.go` | Names contain only `[a-z0-9-]`, start with `[a-z]`, end with `[a-z0-9]` | Entity creation |
| `check_vnc_token_security.go` | VNC token single-use, expired tokens rejected | VNC access |
| `check_prod_delete_code.go` | Production VM delete requires valid confirmation code | Delete API call |
| `check_confirmation_rate_limit.go` | Max 3 failed confirmation attempts per user per 5 minutes | Delete API call |

---

### New Schemas Required

| Schema File | Key Fields | Purpose |
|-------------|-----------|---------|
| `ent/schema/permission.go` | id, name, resource, action | Atomic permission definitions |
| `ent/schema/role.go` | id, name, is_builtin, permissions (edge) | Role = bundle of permissions |
| `ent/schema/role_binding.go` | user_id, role_id, scope_type, scope_id, allowed_environments | User-role assignments with scope |
| `ent/schema/idp_config.go` | name, type, issuer, claims_mapping, default_role_id | IdP configuration |
| `ent/schema/idp_synced_group.go` | idp_config_id, group_id, group_name, source_field | Synced groups from IdP |
| `ent/schema/idp_group_mapping.go` | idp_config_id, idp_group_id, role_id, allowed_environments | IdP group → Shepherd role mapping |
| `ent/schema/notification.go` | recipient, type, title, content, related_ticket_id, read, read_at | Internal inbox notifications |
| `ent/schema/approval_policy.go` | name, environment, operation, requires_approval, approvers, priority | Configurable approval rules (future) |
| `ent/schema/batch_approval_ticket.go` | ticket_id, batch_type, child_count, success_count, failed_count, status | Parent ticket for batch operations |
| `ent/schema/child_approval_ticket.go` | ticket_id, parent_ticket_id, vm_spec, status, error_message | Child ticket for individual items |
| `ent/schema/vnc_access_token.go` | token_id, vm_id, user_id, ticket_id, expires_at, used_at, revoked_at | VNC temporary access tokens |
| `ent/schema/delete_confirmation_code.go` | code, entity_type, entity_id, user_id, expires_at | Production delete confirmation codes |
| `ent/schema/rate_limit_counter.go` | user_id, limit_type, current_value, window_start, exempted | User rate limit counters (PostgreSQL-based) |
| `ent/schema/rate_limit_exemption.go` | user_id, exempted_by, exempted_at, reason, expires_at | Admin-granted rate limit exemptions |

---

### Schema Field Changes Summary

#### System Entity

| Field | Change |
|-------|--------|
| `namespace` | **REMOVE** |
| `environment` | **REMOVE** |
| `maintainers` | **NOT ADDED** - permissions managed via RoleBinding table |
| `tenant_id` | **ADD** `string` (default: "default", reserved for multi-tenancy) |
| Index | Change to `name` only (global unique) |

#### Service Entity

| Field | Change |
|-------|--------|
| `created_by` | **REMOVE** (inherit from System) |
| `maintainers` | **REMOVE** (inherit from System) |

#### VM Entity

| Field | Change |
|-------|--------|
| `system_id` (in VM struct) | **REMOVE COMPLETELY** - query via `vm.Edges.Service.Edges.System`. Do not keep as convenience field to prevent inconsistency. |
| `system_id` (in VMSpec struct) | **REMOVE COMPLETELY** - same rationale |

#### Template Entity

| Field | Change |
|-------|--------|
| `quick_fields` | **ADD** JSON field for quick mode config |
| `advanced_fields` | **ADD** JSON field for advanced mode config |
| `field_defaults` | **ADD** JSON field for default values |
| `field_constraints` | **ADD** JSON field for min/max/options |

#### Cluster Entity

| Field | Change |
|-------|--------|
| `storage_classes` | **ADD** `[]string` (auto-detected) |
| `default_storage_class` | **ADD** `string` (admin-selected) |
| `storage_classes_updated_at` | **ADD** `time.Time` |

#### ApprovalTicket Entity

| Field | Change |
|-------|--------|
| `template_version` | **ADD** `int` |
| `template_snapshot` | **ADD** `text` |
| `selected_storage_class` | **ADD** `string` (optional) |

---

## References

- [ADR-0014: Capability Detection](./ADR-0014-capability-detection.md) - Template capability requirements
- [ADR-0009: Domain Event Pattern](./ADR-0009-domain-event-pattern.md) - Audit trail foundation
- [ADR-0005: Workflow Extensibility](./ADR-0005-workflow-extensibility.md) - Approval workflow baseline
- [RFC-0002: Temporal Integration](../rfc/RFC-0002-temporal.md) - Future complex workflow support
- [RFC-0011: VNC Console](../rfc/RFC-0011-vnc-console.md) - VNC implementation details
- [Phase 1: Core Contracts](../design/phases/01-contracts.md) - Original schema designs (to be updated)
- [Phase 4: Governance](../design/phases/04-governance.md) - Approval workflow details (to be updated)

