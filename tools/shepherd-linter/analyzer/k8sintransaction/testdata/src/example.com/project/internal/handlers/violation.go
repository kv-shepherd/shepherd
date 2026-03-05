// Package handlers contains a violation: K8s provider call inside a transaction callback.
package handlers

import "fmt"

// fakeProvider simulates a K8s provider with methods that make real API calls.
type fakeProvider struct{}

func (p *fakeProvider) CreateVM() error { return nil }

// fakeEntClient simulates an Ent DB client.
type fakeEntClient struct{}

// WithTx simulates the transaction helper that takes a callback closure.
func (c *fakeEntClient) WithTx(fn func() error) error {
	return fn()
}

var db = &fakeEntClient{}
var provider = &fakeProvider{}

// CreateVMHandler violates ADR-0006: calls K8s provider inside a DB transaction.
func CreateVMHandler() error {
	return db.WithTx(func() error {
		err := provider.CreateVM() // want `suspicious K8s API call CreateVM\(\) inside transaction callback`
		if err != nil {
			fmt.Println(err)
		}
		return nil
	})
}
