// Package exempt is the clean testdata fixture for the ssacompliance analyzer.
// It simulates an exempt file (mapper.go-equivalent) that legitimately references
// KubeVirt types for read-only mapping — must produce zero diagnostics.
package exempt

import "fmt"

// fakeTypedClient simulates a read-only client in an exempt file.
type fakeTypedClient struct{}

// Get simulates a read-only operation (allowed).
func (c *fakeTypedClient) Get() string { return "vm-data" }

var vm = &fakeTypedClient{}

// MapVM is a legitimate read-only mapping operation — not a write path.
func MapVM() {
	data := vm.Get()
	fmt.Println(data)
}
