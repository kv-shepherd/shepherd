package handlers // want `RBAC policy: server_namespace.go missing required guard requireActorWithAnyGlobalPermission\(..., "cluster:read", "cluster:write"\)` `RBAC policy: server_namespace.go missing required guard requireActorWithAnyGlobalPermission\(..., "cluster:write"\)`

func badNamespace() {
	middleware.GetUserID(nil) // want `RBAC policy: legacy middleware.GetUserID access is forbidden in server_namespace.go`
}

type middlewarePackage struct{}

var middleware middlewarePackage

func (middlewarePackage) GetUserID(any) string { return "" }
