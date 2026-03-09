// Package server contains clean code (no MockProvider in runtime).
package server

import "fmt"

// Start simulates a real server start with real provider.
func Start() { fmt.Println("starting") }
