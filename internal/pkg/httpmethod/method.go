package httpmethod

import (
	"net/http"
	"strings"
)

const (
	// Other is the OpenTelemetry semantic convention fallback for unknown HTTP methods.
	Other = "_OTHER"

	methodQuery = "QUERY"
)

// NormalizeForObservability returns a low-cardinality HTTP method value for metrics,
// logs, and traces.
func NormalizeForObservability(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case http.MethodConnect,
		http.MethodDelete,
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodPatch,
		http.MethodPost,
		http.MethodPut,
		methodQuery,
		http.MethodTrace:
		return method
	default:
		return Other
	}
}
