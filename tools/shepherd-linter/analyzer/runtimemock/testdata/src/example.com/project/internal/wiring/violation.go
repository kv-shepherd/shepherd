// Package wiring contains a violation: runtime wiring of MockProvider.
package wiring

import "fmt"

// NewMockProvider simulates the test-only constructor being called at runtime.
func NewMockProvider() string { return "mock" }

// wireProviders is a simulated runtime wiring function.
func wireProviders() {
	p := NewMockProvider() // want `runtime wiring must not call NewMockProvider\(\)`
	fmt.Println(p)
}
