package service

import "testing"

func TestNormalizeTemplateSourceType_RejectsLegacyAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "containerdisk canonical", in: "containerdisk", want: TemplateSourceContainerDisk},
		{name: "cdi image import canonical", in: "cdi_image_import", want: TemplateSourceCDIImageImport},
		{name: "cdi pvc clone canonical", in: "cdi_pvc_clone", want: TemplateSourceCDIPVCClone},
		{name: "legacy image alias no longer normalized", in: "image", want: "image"},
		{name: "legacy pvc alias no longer normalized", in: "pvc", want: "pvc"},
		{name: "legacy registry alias no longer normalized", in: "registry", want: "registry"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeTemplateSourceType(tt.in); got != tt.want {
				t.Fatalf("NormalizeTemplateSourceType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsValidTemplateSourceType_CanonicalOnly(t *testing.T) {
	t.Parallel()

	valid := []string{
		TemplateSourceContainerDisk,
		TemplateSourceCDIImageImport,
		TemplateSourceCDIPVCClone,
	}
	for _, sourceType := range valid {
		if !IsValidTemplateSourceType(sourceType) {
			t.Fatalf("IsValidTemplateSourceType(%q) = false, want true", sourceType)
		}
	}

	invalid := []string{"image", "pvc", "registry", "unknown"}
	for _, sourceType := range invalid {
		if IsValidTemplateSourceType(sourceType) {
			t.Fatalf("IsValidTemplateSourceType(%q) = true, want false", sourceType)
		}
	}
}

func TestEffectiveTemplateSourceType_CanonicalOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sourceType string
		imageURL   string
		pvcName    string
		want       string
	}{
		{name: "canonical containerdisk", sourceType: TemplateSourceContainerDisk, imageURL: "quay.io/demo/os:latest", want: TemplateSourceContainerDisk},
		{name: "canonical image import", sourceType: TemplateSourceCDIImageImport, imageURL: "quay.io/demo/os:latest", want: TemplateSourceCDIImageImport},
		{name: "canonical pvc clone", sourceType: TemplateSourceCDIPVCClone, pvcName: "golden-root", want: TemplateSourceCDIPVCClone},
		{name: "legacy image alias rejected", sourceType: "image", imageURL: "quay.io/demo/os:latest", want: ""},
		{name: "legacy registry alias rejected", sourceType: "registry", imageURL: "quay.io/demo/os:latest", want: ""},
		{name: "legacy pvc alias rejected", sourceType: "pvc", pvcName: "golden-root", want: ""},
		{name: "implicit image url fallback rejected", imageURL: "quay.io/demo/os:latest", want: ""},
		{name: "implicit pvc fallback rejected", pvcName: "golden-root", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveTemplateSourceType(tt.sourceType, tt.imageURL, tt.pvcName); got != tt.want {
				t.Fatalf("EffectiveTemplateSourceType(%q, %q, %q) = %q, want %q", tt.sourceType, tt.imageURL, tt.pvcName, got, tt.want)
			}
		})
	}
}

func TestResolveTemplateBootTransport_CanonicalOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sourceType   string
		imageURL     string
		pvcName      string
		pvcNamespace string
		want         string
		wantErr      bool
	}{
		{name: "canonical image import uses cdi import", sourceType: TemplateSourceCDIImageImport, imageURL: "https://images.example.com/os.qcow2", want: "import-image:https://images.example.com/os.qcow2"},
		{name: "explicit containerdisk stays direct", sourceType: TemplateSourceContainerDisk, imageURL: "quay.io/demo/os:latest", want: "quay.io/demo/os:latest"},
		{name: "legacy alias rejected", sourceType: "image", imageURL: "quay.io/demo/os:latest", wantErr: true},
		{name: "missing source type rejected", imageURL: "quay.io/demo/os:latest", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveTemplateBootTransport(tt.sourceType, tt.imageURL, tt.pvcName, tt.pvcNamespace)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ResolveTemplateBootTransport() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveTemplateBootTransport() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveTemplateBootTransport() = %q, want %q", got, tt.want)
			}
		})
	}
}
