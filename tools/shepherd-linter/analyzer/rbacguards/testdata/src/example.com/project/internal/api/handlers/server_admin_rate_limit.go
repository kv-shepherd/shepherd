package handlers

func badRateLimit() {
	requirePlatformAdminActor(nil) // want `RBAC policy: legacy requirePlatformAdminActor helper is forbidden`
}

func requirePlatformAdminActor(any) {}
