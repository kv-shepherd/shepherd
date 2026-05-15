package k8spollingrv_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/k8spollingrv"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	// Run on both polling and non-polling packages.
	// The polling package (provider/vm_status_sync.go) should have violations.
	// The non-polling package (handlers/server_vm.go) should NOT.
	analysistest.Run(t, testdata, k8spollingrv.Analyzer,
		"example.com/project/internal/provider",
		"example.com/project/internal/handlers",
		"kv-shepherd.io/shepherd/internal/api/handlers",
	)
}
