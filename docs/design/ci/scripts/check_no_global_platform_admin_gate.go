//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	var failures []string

	routerPath := "internal/app/router.go"
	routerRaw, err := os.ReadFile(routerPath)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", routerPath, err)
		os.Exit(1)
	}
	router := string(routerRaw)
	if strings.Contains(router, "RequirePermission(\"platform:admin\")") {
		failures = append(failures, fmt.Sprintf("%s contains route-level global platform:admin gate", routerPath))
	}
	if strings.Contains(router, "rbacAdminRoutes(") {
		failures = append(failures, fmt.Sprintf("%s still wires rbacAdminRoutes middleware", routerPath))
	}

	rateLimitPath := "internal/api/handlers/server_admin_rate_limit.go"
	rateLimitRaw, err := os.ReadFile(rateLimitPath)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", rateLimitPath, err)
		os.Exit(1)
	}
	rateLimit := string(rateLimitRaw)
	if strings.Contains(rateLimit, "requirePlatformAdminActor(") {
		failures = append(failures, fmt.Sprintf("%s still uses legacy requirePlatformAdminActor helper", rateLimitPath))
	}

	if len(failures) > 0 {
		fmt.Println("FAIL: global platform-admin gate check failed")
		for _, item := range failures {
			fmt.Printf(" - %s\n", item)
		}
		fmt.Println("Rule: admin authorization must be handler-level, explicit, and permission-granular.")
		os.Exit(1)
	}

	fmt.Println("OK: no global platform-admin route gate detected")
}
