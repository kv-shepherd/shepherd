package handlers // want `RBAC policy: member.go missing required guard requireActorWithAnyGlobalPermission\(..., "user:manage", "rbac:read", "rbac:manage"\)`

func member() {}
