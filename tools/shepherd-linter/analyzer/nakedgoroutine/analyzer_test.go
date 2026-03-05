package nakedgoroutine_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/nakedgoroutine"
)

func TestNakedGoroutineAnalyzer(t *testing.T) {
	// analysistest.TestData() resolves to the "testdata" directory relative to
	// the test package directory (where go test runs). Per pkg.go.dev best practice.
	analysistest.Run(t, analysistest.TestData(), nakedgoroutine.Analyzer,
		"example.com/project/internal/service")
}
