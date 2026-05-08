package service

import (
	"os"
	"strings"
	"testing"
)

func TestVMInstanceAllocationStaysOutOfServicePackage(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read service package directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		source := string(data)
		if strings.Contains(source, "VMNamingService") || strings.Contains(source, "GenerateVMName") {
			t.Fatalf("%s reintroduces service-layer VM instance allocation; use sqlc AllocateServiceInstance", entry.Name())
		}
	}
}
