package main

import (
	"strings"
	"testing"
)

func TestBuildReportInstancesize(t *testing.T) {
	report, err := buildReport("instancesize")
	if err != nil {
		t.Fatalf("buildReport(instancesize) error = %v", err)
	}
	if report.FromVersion != "1.7.0" {
		t.Fatalf("FromVersion = %q, want %q", report.FromVersion, "1.7.0")
	}
	if report.ToVersion != "1.8.0" {
		t.Fatalf("ToVersion = %q, want %q", report.ToVersion, "1.8.0")
	}
	if len(report.SchemaPathsAdded) == 0 {
		t.Fatal("SchemaPathsAdded is empty, want non-empty candidate set")
	}

	found := false
	for _, item := range report.SchemaPathsAdded {
		if item.Path != "spec.template.spec.domain.rebootPolicy" {
			continue
		}
		found = true
		if item.IntroducedIn != "1.8.0" {
			t.Fatalf("rebootPolicy introduced_in = %q, want %q", item.IntroducedIn, "1.8.0")
		}
		if item.SuggestedDisplay == "" || item.SuggestedHelp == "" || item.SuggestedPlacehold == "" {
			t.Fatal("rebootPolicy suggestions should not be empty")
		}
	}
	if !found {
		t.Fatal("expected rebootPolicy to appear in schema upgrade candidates")
	}
}

func TestBuildSuggestedLocaleKey(t *testing.T) {
	got := buildSuggestedLocaleKey("spec.template.spec.domain.devices.interfaces[].passtBinding", "_help")
	want := "instanceSizes.mask.spec_template_spec_domain_devices_interfaces_items_passt_binding_help"
	if got != want {
		t.Fatalf("buildSuggestedLocaleKey(...) = %q, want %q", got, want)
	}
}

func TestAuditMaskLocaleKeysReportsMissingLocaleEntries(t *testing.T) {
	fields := []maskField{
		{
			Path:           "spec.template.spec.domain.rebootPolicy",
			DisplayNameKey: "instanceSizes.mask.reboot_policy",
			HelpKey:        "instanceSizes.mask.reboot_policy_help",
		},
	}
	en := map[string]string{
		"instanceSizes.mask.reboot_policy": "Reboot Policy",
	}
	zh := map[string]string{
		"instanceSizes.mask.reboot_policy": "重启策略",
	}

	gaps := auditMaskLocaleKeys(fields, en, zh)
	if len(gaps) != 1 {
		t.Fatalf("auditMaskLocaleKeys(...) returned %d gaps, want 1", len(gaps))
	}
	if gaps[0].Key != "instanceSizes.mask.reboot_policy_help" {
		t.Fatalf("gap key = %q, want help key", gaps[0].Key)
	}
	if !gaps[0].MissingEN || !gaps[0].MissingZH {
		t.Fatalf("gap missing flags = en:%v zh:%v, want both true", gaps[0].MissingEN, gaps[0].MissingZH)
	}
}

func TestPrintReportIncludesSections(t *testing.T) {
	report := &upgradeReport{
		Entity:      "instancesize",
		FromVersion: "1.7.0",
		ToVersion:   "1.8.0",
		SchemaPathsAdded: []candidateField{{
			Path:         "spec.template.spec.domain.rebootPolicy",
			IntroducedIn: "1.8.0",
		}},
	}

	var builder strings.Builder
	printTo(&builder, report)
	output := builder.String()
	if !strings.Contains(output, "Schema additions not yet exposed in mask") {
		t.Fatal("expected output to include schema additions section")
	}
	if !strings.Contains(output, "v1.8.0+") {
		t.Fatal("expected output to include introduced-in badge text")
	}
}
