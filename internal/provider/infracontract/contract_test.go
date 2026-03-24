package infracontract

import "testing"

func TestListOptionsZeroValue(t *testing.T) {
	var options ListOptions
	if options.Limit != 0 {
		t.Fatalf("ListOptions.Limit = %d, want 0", options.Limit)
	}
}
