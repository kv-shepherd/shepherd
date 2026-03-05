// Package provider is a test fixture for k8spollingrv analyzer.
// File name: vm_status_sync.go — triggers isPollingRelatedFile.
package provider

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func pollWithoutRV() {
	_ = metav1.ListOptions{}          // want `ADR-0038: ListOptions literal missing ResourceVersion field`
	_ = metav1.GetOptions{}           // want `ADR-0038: GetOptions literal missing ResourceVersion field`
	_ = metav1.ListOptions{Limit: 10} // want `ADR-0038: ListOptions literal missing ResourceVersion field`
}

func pollWithRV() {
	_ = metav1.ListOptions{ResourceVersion: "12345"} // OK: ResourceVersion set
	_ = metav1.GetOptions{ResourceVersion: "67890"}  // OK: ResourceVersion set
	_ = metav1.ListOptions{
		ResourceVersion: "0",
		Limit:           10,
	} // OK: ResourceVersion set
}
