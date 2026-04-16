package rbacguards_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/rbacguards"
)

func TestRBACGuardsAnalyzer(t *testing.T) {
	analysistest.Run(
		t,
		analysistest.TestData(),
		rbacguards.Analyzer,
		"example.com/project/internal/app",
		"example.com/project/internal/api/handlers",
	)
}
