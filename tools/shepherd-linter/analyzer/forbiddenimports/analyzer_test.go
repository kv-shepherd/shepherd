package forbiddenimports_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/forbiddenimports"
)

func TestForbiddenImportsAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	// clean package: outside enforced path, no violations expected
	analysistest.Run(t, testdata, forbiddenimports.Analyzer, "forbiddenimports")
	// violation package: under internal/, hardcoded path and outbox imports must be flagged
	analysistest.Run(t, testdata, forbiddenimports.Analyzer,
		"example.com/project/internal/api")
	// violation package: outbox package path itself is forbidden
	analysistest.Run(t, testdata, forbiddenimports.Analyzer,
		"example.com/project/internal/outbox")
}
