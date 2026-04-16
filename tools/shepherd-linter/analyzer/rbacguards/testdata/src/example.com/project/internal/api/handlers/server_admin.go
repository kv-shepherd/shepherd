package handlers // want `RBAC policy: server_admin.go missing required guard requireAnyGlobalPermission\(..., "vm:create", "template:read", "template:write"\)` `RBAC policy: server_admin.go missing required guard requireAnyGlobalPermission\(..., "vm:create", "instance_size:read", "instance_size:write"\)`

func admin() {}
