//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	workerFile   = "internal/jobs/vm_create.go"
	providerFile = "internal/provider/kubevirt.go"
)

func main() {
	var violations []string

	workerSrc, err := os.ReadFile(workerFile)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", workerFile, err)
		os.Exit(1)
	}
	workerText := string(workerSrc)

	requiredWorkerFragments := []string{
		"specOverrides := resolveInstanceSizeSpecOverrides(",
		"SpecOverrides: specOverrides,",
		"spec.SpecOverrides = applySpecOverridePatches(",
		"extractSpecOverridesFromModifiedSpec(",
	}
	for _, fragment := range requiredWorkerFragments {
		if !strings.Contains(workerText, fragment) {
			violations = append(violations, fmt.Sprintf("%s: missing %q", workerFile, fragment))
		}
	}

	providerSrc, err := os.ReadFile(providerFile)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", providerFile, err)
		os.Exit(1)
	}
	providerText := string(providerSrc)

	requiredProviderFragments := []string{
		"func applySpecOverrides(",
		"applySpecOverrides(vm, spec.SpecOverrides)",
		`invalid spec_overrides path`,
	}
	for _, fragment := range requiredProviderFragments {
		if !strings.Contains(providerText, fragment) {
			violations = append(violations, fmt.Sprintf("%s: missing %q", providerFile, fragment))
		}
	}

	if len(violations) > 0 {
		fmt.Println("FAIL: vm_create spec completeness check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: Stage 5.C must carry InstanceSize spec_overrides through Worker -> VMSpec -> Provider render.")
		os.Exit(1)
	}

	fmt.Println("OK: vm_create spec completeness check passed")
}
