//go:build ignore

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	workerFile   = "internal/jobs/vm_create.go"
	serviceFile  = "internal/service/vm_service.go"
	providerFile = "internal/provider/kubevirt.go"
	rendererFile = "internal/provider/vm_renderer.go"
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
		"extractSpecOverridesFromModifiedSpec(",
	}
	for _, fragment := range requiredWorkerFragments {
		if !strings.Contains(workerText, fragment) {
			violations = append(violations, fmt.Sprintf("%s: missing %q", workerFile, fragment))
		}
	}
	requiredWorkerPatterns := map[string]*regexp.Regexp{
		"SpecOverrides: specOverrides,": regexp.MustCompile(`SpecOverrides:\s*specOverrides,`),
	}
	for label, pattern := range requiredWorkerPatterns {
		if !pattern.MatchString(workerText) {
			violations = append(violations, fmt.Sprintf("%s: missing %q", workerFile, label))
		}
	}

	serviceSrc, err := os.ReadFile(serviceFile)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", serviceFile, err)
		os.Exit(1)
	}
	serviceText := string(serviceSrc)

	// ADR-0011: Render responsibility sits in service layer (ensureRenderedYAML),
	// provider only consumes rendered YAML and applies via SSA.
	requiredServiceFragments := []string{
		"ensureRenderedYAML(",
		"provider.RenderVMSpecToYAML(",
	}
	for _, fragment := range requiredServiceFragments {
		if !strings.Contains(serviceText, fragment) {
			violations = append(violations, fmt.Sprintf("%s: missing %q", serviceFile, fragment))
		}
	}
	requiredServicePatterns := map[string]*regexp.Regexp{
		"SpecOverrides: spec.SpecOverrides": regexp.MustCompile(`SpecOverrides:\s*spec\.SpecOverrides`),
	}
	for label, pattern := range requiredServicePatterns {
		if !pattern.MatchString(serviceText) {
			violations = append(violations, fmt.Sprintf("%s: missing %q", serviceFile, label))
		}
	}

	providerSrc, err := os.ReadFile(providerFile)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", providerFile, err)
		os.Exit(1)
	}
	providerText := string(providerSrc)

	requiredProviderFragments := []string{
		"spec.rendered_yaml is required (ADR-0011)",
		"client.SSA().ApplyYAML(",
		"client.SSA().DryRunApplyYAML(",
	}
	for _, fragment := range requiredProviderFragments {
		if !strings.Contains(providerText, fragment) {
			violations = append(violations, fmt.Sprintf("%s: missing %q", providerFile, fragment))
		}
	}

	rendererSrc, err := os.ReadFile(rendererFile)
	if err != nil {
		fmt.Printf("FAIL: read %s: %v\n", rendererFile, err)
		os.Exit(1)
	}
	rendererText := string(rendererSrc)

	// ADR-0011 + ADR-0018: YAML renderer must apply SpecOverrides and validate paths.
	requiredRendererFragments := []string{
		"applySpecOverridesToYAML(",
		"SpecOverrides map[string]interface{}",
		`invalid spec_overrides path`,
	}
	for _, fragment := range requiredRendererFragments {
		if !strings.Contains(rendererText, fragment) {
			violations = append(violations, fmt.Sprintf("%s: missing %q", rendererFile, fragment))
		}
	}

	if len(violations) > 0 {
		fmt.Println("FAIL: vm_create spec completeness check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: Stage 5.C must carry InstanceSize spec_overrides through Worker -> VMSpec -> Service render -> Provider SSA apply (ADR-0011/0018).")
		os.Exit(1)
	}

	fmt.Println("OK: vm_create spec completeness check passed")
}
