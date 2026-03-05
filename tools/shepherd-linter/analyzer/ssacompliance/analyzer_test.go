package ssacompliance_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/ssacompliance"
)

func TestSSAComplianceAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	// Run against the provider package (violation + exempt cases)
	analysistest.Run(t, testdata, ssacompliance.Analyzer,
		"example.com/project/internal/provider",
	)
}

func TestSSAComplianceAnalyzer_ExemptFile(t *testing.T) {
	testdata := analysistest.TestData()
	// Exempt package (mapper) — must produce zero diagnostics
	analysistest.Run(t, testdata, ssacompliance.Analyzer,
		"example.com/project/internal/provider/exempt",
	)
}
