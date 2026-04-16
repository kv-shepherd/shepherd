package manualdi_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/manualdi"
)

func TestManualDIAnalyzer(t *testing.T) {
	analysistest.Run(
		t,
		analysistest.TestData(),
		manualdi.Analyzer,
		"example.com/project/internal/api",
		"example.com/project/internal/app",
	)
}
