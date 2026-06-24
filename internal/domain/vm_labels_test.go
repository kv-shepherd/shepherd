package domain

import (
	"strings"
	"testing"
)

func TestShepherdVMLabelKeys(t *testing.T) {
	tests := map[string]string{
		"service ID":  ShepherdServiceIDLabel,
		"template ID": ShepherdTemplateIDLabel,
		"event ID":    ShepherdEventIDLabel,
	}
	for name, got := range tests {
		t.Run(name, func(t *testing.T) {
			if got == "" {
				t.Fatal("label key is empty")
			}
			if !strings.HasPrefix(got, "shepherd.io/") {
				t.Fatalf("label key = %q, want shepherd.io/ prefix", got)
			}
		})
	}
}
