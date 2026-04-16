package app

func badRouter() {
	RequirePermission("platform:admin") // want `RBAC policy: route-level global platform:admin gate is forbidden`
	rbacAdminRoutes()                   // want `RBAC policy: legacy rbacAdminRoutes middleware is forbidden`
}

func RequirePermission(string) {}

func rbacAdminRoutes() {}
