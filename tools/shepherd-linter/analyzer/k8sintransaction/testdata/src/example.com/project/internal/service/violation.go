// Package service contains a violation: K8s provider call inside a transaction callback.
package service

import "fmt"

// fakeProvider simulates a K8s provider in the service layer.
type fakeProvider struct{}

func (p *fakeProvider) DeleteVM() error { return nil }

// fakeEntClient simulates an Ent DB client.
type fakeEntClient struct{}

func (c *fakeEntClient) WithTx(fn func() error) error {
	return fn()
}

var db = &fakeEntClient{}
var provider = &fakeProvider{}

// DeleteVMService violates ADR-0006: calls K8s DeleteVM inside DB transaction.
func DeleteVMService() error {
	return db.WithTx(func() error {
		err := provider.DeleteVM() // want `suspicious K8s API call DeleteVM\(\) inside transaction callback`
		if err != nil {
			fmt.Println(err)
		}
		return nil
	})
}
