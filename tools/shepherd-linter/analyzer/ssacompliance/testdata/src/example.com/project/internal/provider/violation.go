// Package provider is the violation testdata fixture for the ssacompliance analyzer.
// It simulates a provider write path that calls forbidden methods.
package provider

import "fmt"

// fakeTypedClient simulates a KubeVirt typed client with forbidden Write methods.
type fakeTypedClient struct{}

// Create simulates the forbidden typed-client Create().
func (c *fakeTypedClient) Create() error { return nil }

// Update simulates the forbidden typed-client Update().
func (c *fakeTypedClient) Update() error { return nil }

// vm simulates a typed client instance in a provider write path.
var vm = &fakeTypedClient{}

// WriteVMCreate violates ADR-0011: calls typed-client .Create() directly.
func WriteVMCreate() {
	err := vm.Create() // want `typed-client .Create\(\) is forbidden`
	if err != nil {
		fmt.Println(err)
	}
}

// WriteVMUpdate violates ADR-0011: calls typed-client .Update() directly.
func WriteVMUpdate() {
	err := vm.Update() // want `typed-client .Update\(\) is forbidden`
	if err != nil {
		fmt.Println(err)
	}
}
