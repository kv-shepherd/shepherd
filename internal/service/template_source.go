package service

import (
	"fmt"
	"strings"
)

const (
	TemplateSourceContainerDisk  = "containerdisk"
	TemplateSourceCDIImageImport = "cdi_image_import"
	TemplateSourceCDIPVCClone    = "cdi_pvc_clone"
)

const (
	templateTransportClonePVC    = "clone-pvc:"
	templateTransportImportImage = "import-image:"
)

// NormalizeTemplateSourceType normalizes source_type input to the canonical boot
// source kinds accepted by the platform.
//
// Pre-launch cleanup: the project no longer accepts legacy aliases such as
// "image", "registry", or "pvc". New and existing code paths must use one of:
//   - "containerdisk"
//   - "cdi_image_import"
//   - "cdi_pvc_clone"
//
// Empty string is preserved so draft templates can remain partially configured.
func NormalizeTemplateSourceType(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "":
		return ""
	case TemplateSourceContainerDisk:
		return TemplateSourceContainerDisk
	case TemplateSourceCDIImageImport:
		return TemplateSourceCDIImageImport
	case TemplateSourceCDIPVCClone:
		return TemplateSourceCDIPVCClone
	default:
		return strings.TrimSpace(strings.ToLower(raw))
	}
}

// EffectiveTemplateSourceType returns the canonical boot source kind for a template.
// The platform is pre-launch and does not retain legacy source_type compatibility.
// Runtime paths must rely on the explicit canonical source_type field.
func EffectiveTemplateSourceType(sourceType, imageURL, pvcName string) string {
	_ = imageURL
	_ = pvcName

	switch NormalizeTemplateSourceType(sourceType) {
	case TemplateSourceContainerDisk:
		return TemplateSourceContainerDisk
	case TemplateSourceCDIImageImport:
		return TemplateSourceCDIImageImport
	case TemplateSourceCDIPVCClone:
		return TemplateSourceCDIPVCClone
	default:
		return ""
	}
}

func IsValidTemplateSourceType(raw string) bool {
	switch NormalizeTemplateSourceType(raw) {
	case TemplateSourceContainerDisk, TemplateSourceCDIImageImport, TemplateSourceCDIPVCClone:
		return true
	default:
		return false
	}
}

// IsUserRequestableTemplateSource reports whether a template boot source should
// be exposed in the normal VM request flow. The request flow is intentionally
// restricted to persistent-root semantics; ephemeral containerdisk templates
// remain admin-maintained but are not user-requestable.
func IsUserRequestableTemplateSource(sourceType, imageURL, pvcName string) bool {
	switch EffectiveTemplateSourceType(sourceType, imageURL, pvcName) {
	case TemplateSourceCDIImageImport, TemplateSourceCDIPVCClone:
		return true
	default:
		return false
	}
}

// ResolveTemplateBootTransport maps a template's persisted semantic fields into
// the internal boot source transport string consumed by VM create / dry-run paths.
func ResolveTemplateBootTransport(sourceType, imageURL, pvcName, pvcNamespace string) (string, error) {
	switch EffectiveTemplateSourceType(sourceType, imageURL, pvcName) {
	case TemplateSourceContainerDisk:
		if strings.TrimSpace(imageURL) == "" {
			return "", fmt.Errorf("image_url is required when source_type is %q", TemplateSourceContainerDisk)
		}
		return strings.TrimSpace(imageURL), nil
	case TemplateSourceCDIImageImport:
		normalizedURL, err := normalizeCDIImportURL(imageURL)
		if err != nil {
			return "", err
		}
		return templateTransportImportImage + normalizedURL, nil
	case TemplateSourceCDIPVCClone:
		name := strings.TrimSpace(pvcName)
		if name == "" {
			return "", fmt.Errorf("pvc_name is required when source_type is %q", TemplateSourceCDIPVCClone)
		}
		if ns := strings.TrimSpace(pvcNamespace); ns != "" {
			return templateTransportClonePVC + ns + "/" + name, nil
		}
		return templateTransportClonePVC + name, nil
	case "":
		return "", fmt.Errorf("template source is not configured")
	default:
		return "", fmt.Errorf("unsupported source_type %q", sourceType)
	}
}

func normalizeCDIImportURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("image_url is required when source_type is %q", TemplateSourceCDIImageImport)
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "docker://"):
		return trimmed, nil
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return trimmed, nil
	case strings.Contains(trimmed, "://"):
		return "", fmt.Errorf("unsupported cdi image import URL %q", trimmed)
	default:
		// CDI registry imports use docker:// URLs. Accept plain registry refs and
		// canonicalize them here so callers can keep using familiar image notation.
		return "docker://" + trimmed, nil
	}
}
