# RFC-0011: VNC Console (noVNC)

> **Status**: Proposed (V1 simplified implementation)  
> **Priority**: P1  
> **Trigger**: ~~Browser-based VM console access required~~ V1 core feature

---

## Scope Clarification

> ⚠️ **Note**: VNC security specifications have been accepted as part of [ADR-0015: Governance Model V2](../adr/ADR-0015-governance-model-v2.md) §18 (VNC Console Access Permissions).
>
> **ADR-0015 defines (must be followed):**
>
> | Specification | ADR-0015 Location |
> |---------------|-------------------|
> | Permission Matrix (test/prod) | §18 Table |
> | Token Security (single-use, time-bounded, user-binding) | §18 Token Structure |
> | Encryption Key Management | §18 shared with cluster credentials |
> | Audit Logging Requirements | §18 Audit Table |
> | V1 delivery scope freeze (no active revoke API) | §18.1 Addendum |
>
> **This RFC covers frontend implementation only:**
> - noVNC JavaScript library integration
> - WebSocket proxy implementation
> - UI/UX for console access
>
> All security and permission logic must conform to ADR-0015 §18 and §18.1 addendum.

---

## V1 Implementation Scope

> **V1 adopts a simplified implementation** to balance feature delivery with complexity.

| Feature | V1 (Simplified) | Full (V2+) |
|---------|-----------------|------------|
| Token storage | Signed JWT + shared replay marker (`jti`, `used_at`) | VNCAccessToken table |
| Token TTL | 2 hours (ADR-0015) | Configurable |
| Token revocation | Short TTL only | Active revocation API |
| Session recording | ❌ Not supported | ✅ Optional |
| Test env approval | Skip (RBAC check only) | Configurable |
| Prod env approval | Required | Required |

> **Clarification (ADR-0015 §18.1 Addendum)**:
> V1 single-use enforcement still requires a shared replay marker (`jti`, `used_at`) across replicas.
> V1 does not expose an active token revocation API.

### V1 API Endpoint

```
# Request console access (prod: approval ticket, test: direct URL)
POST /api/v1/vms/{vm_id}/console/request

# Poll approval/access status
GET /api/v1/vms/{vm_id}/console/status

# WebSocket endpoint for noVNC token-based access
GET /api/v1/vms/{vm_id}/vnc?token={vnc_jwt}
```

---

## Problem

Users may need to access VM consoles directly from the governance platform UI without additional tools.

---

## Proposed Solution

### Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                       noVNC Integration                              │
│                                                                      │
│  Browser ─────WebSocket────► Shepherd ────► KubeVirt VNC Proxy      │
│                                   │                                  │
│                                   ▼                                  │
│                           subresources/vnc                           │
└─────────────────────────────────────────────────────────────────────┘
```

### WebSocket Proxy

```go
// internal/handler/vnc_handler.go

func (h *VNCHandler) ProxyConsole(c *gin.Context) {
    clusterName := c.Param("cluster")
    namespace := c.Param("namespace")
    vmName := c.Param("name")
    
    // Get cluster config
    cluster, _ := h.clusterService.Get(ctx, clusterName)
    
    // Create VNC stream
    virtClient := h.getClient(cluster)
    stream, _ := virtClient.VirtualMachineInstance(namespace).VNC(vmName)
    
    // Upgrade to WebSocket and proxy
    websocket.Proxy(c.Writer, c.Request, stream)
}
```

### API Endpoint

```
# Public API contract aligns with master-flow Stage 6:
POST /api/v1/vms/{vm_id}/console/request
GET /api/v1/vms/{vm_id}/console/status
GET /api/v1/vms/{vm_id}/vnc?token={vnc_jwt}

# KubeVirt upstream proxy target (internal implementation detail):
# subresources.kubevirt.io/v1/namespaces/{ns}/virtualmachineinstances/{name}/vnc
```

---

## Trigger Conditions

- Users need browser-based console access
- kubectl-based console not acceptable for non-technical users
- Governance platform must provide unified experience

---

## References

- [ADR-0015: Governance Model V2 §18](../adr/ADR-0015-governance-model-v2.md) - VNC security specifications
- [KubeVirt Console Access](https://kubevirt.io/user-guide/virtual_machines/accessing_virtual_machines/)
- [noVNC](https://novnc.com/)
