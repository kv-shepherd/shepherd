// Package service contains a violation: service layer calling Tx().
package service

import "context"

type fakeClient struct{}

func (c *fakeClient) Tx(ctx context.Context) (*fakeClient, error) { return c, nil }

var client = &fakeClient{}

// CreateVM violates the transaction boundary rule.
func CreateVM(ctx context.Context) error {
	_, err := client.Tx(ctx) // want `service layer must not call Tx\(\)`
	return err
}
