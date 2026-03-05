package riverjobargs_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/riverjobargs"
)

func TestRiverJobArgsAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, riverjobargs.Analyzer,
		"example.com/project/internal/usecase")
}
