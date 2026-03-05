package riverbypass_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/riverbypass"
)

func TestRiverBypassAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	// clean package: no violations expected (outside usecase path)
	analysistest.Run(t, testdata, riverbypass.Analyzer, "riverbypass")
	// violation package: under internal/usecase, direct entity writes must be flagged
	analysistest.Run(t, testdata, riverbypass.Analyzer,
		"example.com/project/internal/usecase")
}
