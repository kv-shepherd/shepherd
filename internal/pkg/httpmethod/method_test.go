package httpmethod

import (
	"net/http"
	"testing"
)

func TestNormalizeForObservability(t *testing.T) {
	testCases := []struct {
		name   string
		method string
		want   string
	}{
		{name: "standard", method: http.MethodGet, want: http.MethodGet},
		{name: "lowercase standard", method: "post", want: http.MethodPost},
		{name: "trimmed standard", method: " PATCH ", want: http.MethodPatch},
		{name: "query", method: "QUERY", want: "QUERY"},
		{name: "empty", method: "", want: Other},
		{name: "unknown", method: "BREW", want: Other},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeForObservability(tc.method); got != tc.want {
				t.Fatalf("NormalizeForObservability(%q) = %q, want %q", tc.method, got, tc.want)
			}
		})
	}
}
