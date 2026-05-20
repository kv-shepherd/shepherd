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
	if report.FromVersion != "1.8.1" {
		t.Fatalf("FromVersion = %q, want %q", report.FromVersion, "1.8.1")
	}
	if report.ToVersion != "1.8.2" {
		t.Fatalf("ToVersion = %q, want %q", report.ToVersion, "1.8.2")
	}
	if len(report.SchemaPathsAdded) != 0 {
		t.Fatalf("SchemaPathsAdded length = %d, want 0 for patch baseline refresh", len(report.SchemaPathsAdded))
	}
	if len(report.ChangedMaskedPaths) != 0 {
		t.Fatalf("ChangedMaskedPaths length = %d, want 0 for patch baseline refresh", len(report.ChangedMaskedPaths))
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
		"instanceSizes.mask.reboot_policy": "\u91cd\u542f\u7b56\u7565",
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
		Entity:           "instancesize",
		FromVersion:      "1.8.0",
		ToVersion:        "1.8.1",
		SchemaPathsAdded: nil,
	}

	var builder strings.Builder
	printTo(&builder, report)
	output := builder.String()
	if !strings.Contains(output, "Schema additions not yet exposed in mask") {
		t.Fatal("expected output to include schema additions section")
	}
	if !strings.Contains(output, "baseline: v1.8.0 -> v1.8.1") {
		t.Fatal("expected output to include refreshed baseline header")
	}
}
