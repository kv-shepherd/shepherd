// Package handlers is a test fixture for project provider ListOptions.
// File name intentionally does not match polling patterns; function names do.
package handlers

import (
	"kv-shepherd.io/shepherd/internal/provider"
	infracontract "kv-shepherd.io/shepherd/internal/provider/infracontract"
)

func refreshVMLiveStates() {
	_ = infracontract.ListOptions{} // want `ADR-0038: ListOptions literal missing ResourceVersion field`
	_ = infracontract.ListOptions{ResourceVersion: "12345"}
	_ = provider.ListOptions{} // want `ADR-0038: ListOptions literal missing ResourceVersion field`
	_ = provider.ListOptions{ResourceVersion: ""}
}

func listVMs() {
	_ = infracontract.ListOptions{}
	_ = provider.ListOptions{}
}
