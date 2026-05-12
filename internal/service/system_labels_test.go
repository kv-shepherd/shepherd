package service

import "testing"

func TestNormalizeTemplateSystemLabels(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
	}{
		{name: "empty defaults to any", input: nil, want: []string{SystemLabelOSAny}},
		{name: "normalizes case and order", input: []string{" OS:Windows "}, want: []string{SystemLabelOSWindows}},
		{name: "rejects arbitrary label", input: []string{"os:linux", "freeform"}, wantErr: true},
		{name: "rejects any mixed with concrete", input: []string{SystemLabelOSAny, SystemLabelOSLinux}, wantErr: true},
		{name: "rejects multiple concrete OS requirements", input: []string{SystemLabelOSLinux, SystemLabelOSWindows}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeTemplateSystemLabels(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NormalizeTemplateSystemLabels() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeTemplateSystemLabels() error = %v", err)
			}
			if !stringSlicesEqual(got, tt.want) {
				t.Fatalf("NormalizeTemplateSystemLabels() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeInstanceSizeSystemLabels_AllowsMultipleConcreteOS(t *testing.T) {
	got, err := NormalizeInstanceSizeSystemLabels([]string{SystemLabelOSWindows, SystemLabelOSLinux})
	if err != nil {
		t.Fatalf("NormalizeInstanceSizeSystemLabels() error = %v", err)
	}
	want := []string{SystemLabelOSLinux, SystemLabelOSWindows}
	if !stringSlicesEqual(got, want) {
		t.Fatalf("NormalizeInstanceSizeSystemLabels() = %#v, want %#v", got, want)
	}
}

func TestTemplateInstanceSizeCompatible(t *testing.T) {
	tests := []struct {
		name           string
		templateLabels []string
		sizeLabels     []string
		want           bool
	}{
		{name: "windows template matches windows size", templateLabels: []string{SystemLabelOSWindows}, sizeLabels: []string{SystemLabelOSWindows}, want: true},
		{name: "windows template matches multi OS size", templateLabels: []string{SystemLabelOSWindows}, sizeLabels: []string{SystemLabelOSLinux, SystemLabelOSWindows}, want: true},
		{name: "windows template rejects linux size", templateLabels: []string{SystemLabelOSWindows}, sizeLabels: []string{SystemLabelOSLinux}, want: false},
		{name: "generic template matches linux size", templateLabels: []string{SystemLabelOSAny}, sizeLabels: []string{SystemLabelOSLinux}, want: true},
		{name: "windows template matches generic size", templateLabels: []string{SystemLabelOSWindows}, sizeLabels: []string{SystemLabelOSAny}, want: true},
		{name: "legacy empty labels are generic", templateLabels: nil, sizeLabels: []string{SystemLabelOSLinux}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TemplateInstanceSizeCompatible(tt.templateLabels, tt.sizeLabels); got != tt.want {
				t.Fatalf("TemplateInstanceSizeCompatible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
