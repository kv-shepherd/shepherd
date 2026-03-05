package txboundary_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/txboundary"
)

func TestTxBoundaryAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, txboundary.Analyzer,
		"example.com/project/internal/service")
}
