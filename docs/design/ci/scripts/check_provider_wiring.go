//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	infraFile = "internal/app/modules/infrastructure.go"
	vmFile    = "internal/app/modules/vm.go"
)

func main() {
	var violations []string

	infraSrc, err := os.ReadFile(infraFile)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", infraFile, err)
		os.Exit(1)
	}
	infraText := string(infraSrc)

	if !strings.Contains(infraText, "provider.NewKubeVirtProvider(") {
		violations = append(violations, infraFile+": missing provider.NewKubeVirtProvider() wiring")
	}
	if strings.Contains(infraText, "NewMockProvider(") {
		violations = append(violations, infraFile+": runtime infrastructure must not wire NewMockProvider()")
	}
	if !strings.Contains(infraText, "VMProvider:") {
		violations = append(violations, infraFile+": Infrastructure struct return must assign VMProvider")
	}

	vmSrc, err := os.ReadFile(vmFile)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", vmFile, err)
		os.Exit(1)
	}
	vmText := string(vmSrc)

	if !strings.Contains(vmText, "if infra.VMProvider == nil") {
		violations = append(violations, vmFile+": missing nil-check for infra.VMProvider")
	}
	if !strings.Contains(vmText, `infra.VMProvider.Type() == "mock"`) {
		violations = append(violations, vmFile+`: missing explicit rejection for infra.VMProvider.Type() == "mock"`)
	}
	if !strings.Contains(vmText, "service.NewVMService(infra.VMProvider)") {
		violations = append(violations, vmFile+": vm service must be wired from infra.VMProvider")
	}

	if len(violations) > 0 {
		fmt.Println("FAIL: provider wiring check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: runtime must wire real KubeVirt provider and reject mock provider in module composition root.")
		os.Exit(1)
	}

	fmt.Println("OK: provider wiring check passed")
}
