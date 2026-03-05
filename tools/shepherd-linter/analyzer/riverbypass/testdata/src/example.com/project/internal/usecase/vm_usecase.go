// Package usecase is the violation testdata fixture for the riverbypass analyzer.
// It simulates a UseCase that directly writes to a protected entity without River Queue.
package usecase

import "fmt"

// fakeVMClient simulates an Ent-generated VM entity client.
type fakeVMClient struct{}

// Create simulates a direct DB write.
func (c *fakeVMClient) Create() error { return nil }

// VM simulates the Ent VM entity accessor.
var VM = &fakeVMClient{}

// DirectCreateVM violates ADR-0006: direct Create() on VM without River Queue.
func DirectCreateVM() {
	err := VM.Create() // want `direct write to VM.Create\(\) in UseCase layer`
	if err != nil {
		fmt.Println(err)
	}
}
