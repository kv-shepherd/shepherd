# RFC-0011: VNC Console (noVNC)

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Browser-based VM console access required

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
>
> **This RFC covers frontend implementation only:**
> - noVNC JavaScript library integration
> - WebSocket proxy implementation
> - UI/UX for console access
>
> All security and permission logic must conform to ADR-0015 §18.

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
GET /api/v1/clusters/{cluster}/namespaces/{ns}/vms/{name}/console
Upgrade: websocket
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
