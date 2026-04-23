package handlers // want `RBAC policy: server_namespace.go missing required guard requireActorWithAnyGlobalPermission\(..., "namespace:read", "namespace:write"\)` `RBAC policy: server_namespace.go missing required guard requireActorWithAnyGlobalPermission\(..., "namespace:write"\)`

func badNamespace() {
	middleware.GetUserID(nil) // want `RBAC policy: legacy middleware.GetUserID access is forbidden in server_namespace.go`
}

type middlewarePackage struct{}

var middleware middlewarePackage

func (middlewarePackage) GetUserID(any) string { return "" }
