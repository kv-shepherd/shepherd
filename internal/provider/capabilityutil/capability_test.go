package capabilityutil

import "testing"

func TestHasAllCapabilities(t *testing.T) {
	if !HasAllCapabilities([]string{" VMCreate ", "ExpandDisks"}, []string{"vmcreate"}) {
		t.Fatal("HasAllCapabilities() = false, want true")
	}
	if HasAllCapabilities([]string{"VMCreate"}, []string{"vmcreate", "expanddisks"}) {
		t.Fatal("HasAllCapabilities() = true, want false")
	}
}
