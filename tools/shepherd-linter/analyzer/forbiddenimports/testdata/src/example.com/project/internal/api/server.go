// Package api is the violation testdata fixture for the forbiddenimports analyzer.
// This package is under internal/, so the analyzer will enforce its rules here.
package api

import (
	"fmt"

	_ "example.com/legacy/outbox/client" // want `forbidden import "example.com/legacy/outbox/client"`
)

// badPath contains a hardcoded kubeconfig path, triggering the analyzer.
func badPath() string {
	return "/root/.kube/config" // want `hardcoded path "/root/.kube/config" detected`
}

// clean has no violation.
func clean() { fmt.Println("ok") }
