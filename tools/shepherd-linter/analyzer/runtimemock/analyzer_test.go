package runtimemock_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/runtimemock"
)

func TestRuntimeMockAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	// clean: no MockProvider in runtime pkg
	analysistest.Run(t, testdata, runtimemock.Analyzer, "cmd/server")
	// violation: MockProvider called in runtime wiring
	analysistest.Run(t, testdata, runtimemock.Analyzer,
		"example.com/project/internal/wiring")
}
