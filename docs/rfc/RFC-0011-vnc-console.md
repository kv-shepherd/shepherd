# RFC-0011: VNC Console (noVNC)

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Browser-based VM console access required

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

- [KubeVirt Console Access](https://kubevirt.io/user-guide/virtual_machines/accessing_virtual_machines/)
- [noVNC](https://novnc.com/)
