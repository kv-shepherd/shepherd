package semaphoreusage_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/semaphoreusage"
)

func TestSemaphoreUsageAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	// Both clean and violation are in the same package path
	analysistest.Run(t, testdata, semaphoreusage.Analyzer,
		"example.com/project/internal/service")
}
