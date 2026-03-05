// Package v1 is a minimal stub of k8s.io/apimachinery/pkg/apis/meta/v1
// for analysistest. Only the struct names and fields needed by the analyzer
// are provided.
package v1

// ListOptions is a stub for k8s.io/apimachinery/pkg/apis/meta/v1.ListOptions.
type ListOptions struct {
	ResourceVersion string
	LabelSelector   string
	FieldSelector   string
	Limit           int64
	Continue        string
}

// GetOptions is a stub for k8s.io/apimachinery/pkg/apis/meta/v1.GetOptions.
type GetOptions struct {
	ResourceVersion string
}
