package k8sintransaction_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"kv-shepherd.io/shepherd-linter/analyzer/k8sintransaction"
)

func TestK8sInTransactionAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, k8sintransaction.Analyzer,
		"example.com/project/internal/handlers",
	)
}

func TestK8sInTransactionAnalyzer_ServiceLayer(t *testing.T) {
	testdata := analysistest.TestData()
	// Service layer should also be checked.
	analysistest.Run(t, testdata, k8sintransaction.Analyzer,
		"example.com/project/internal/service",
	)
}

func TestK8sInTransactionAnalyzer_CleanCode(t *testing.T) {
	testdata := analysistest.TestData()
	// Clean code: K8s call outside transaction — must produce zero diagnostics.
	analysistest.Run(t, testdata, k8sintransaction.Analyzer,
		"example.com/project/internal/handlers/clean",
	)
}
