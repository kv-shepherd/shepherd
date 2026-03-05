// Package clean contains clean code: K8s provider call OUTSIDE a transaction callback.
// Must produce zero diagnostics.
package clean

// fakeProvider simulates a K8s provider.
type fakeProvider struct{}

func (p *fakeProvider) CreateVM() error { return nil }

var provider = &fakeProvider{}

// CreateVMClean is correct: K8s call outside transaction.
func CreateVMClean() error {
	// K8s call is outside any DB transaction — this is the correct pattern.
	return provider.CreateVM()
}
