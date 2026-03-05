// Package handlers is a test fixture for k8spollingrv analyzer.
// This file does NOT have a polling-related name, so ListOptions
// without ResourceVersion should NOT be flagged.
package handlers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func listVMs() {
	// Non-polling context: no violation expected.
	_ = metav1.ListOptions{Limit: 100}
	_ = metav1.GetOptions{}
}
