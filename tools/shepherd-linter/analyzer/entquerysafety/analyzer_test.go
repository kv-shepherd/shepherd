package entquerysafety_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/entquerysafety"
)

func TestEntQuerySafetyAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, entquerysafety.Analyzer,
		"example.com/project/internal/api/handlers")
	analysistest.Run(t, testdata, entquerysafety.Analyzer,
		"example.com/project/internal/infrastructure")
}
