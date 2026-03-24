package authproviderlayering_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/authproviderlayering"
)

func TestAuthProviderLayeringAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, authproviderlayering.Analyzer,
		"example.com/project/internal/clean",
		"example.com/project/internal/service",
		"example.com/project/internal/api/handlers",
		"example.com/project/internal/provider",
	)
}
