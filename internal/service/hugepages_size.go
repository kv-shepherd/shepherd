package service

import (
	"fmt"
	"regexp"
	"strings"
)

var hugepagesDisplayPattern = regexp.MustCompile(`^([1-9]\d*)(mi|gi)?$`)

// CanonicalHugepagesPageSize normalizes user-facing hugepages page sizes to the
// KubeVirt-style representation used across API payloads and edit forms.
func CanonicalHugepagesPageSize(in string) string {
	trimmed := strings.TrimSpace(in)
	if trimmed == "" {
		return ""
	}

	compact := strings.ReplaceAll(trimmed, " ", "")
	match := hugepagesDisplayPattern.FindStringSubmatch(strings.ToLower(compact))
	if len(match) != 3 {
		return compact
	}

	unit := match[2]
	switch unit {
	case "", "mi":
		return fmt.Sprintf("%sMi", match[1])
	case "gi":
		return fmt.Sprintf("%sGi", match[1])
	default:
		return compact
	}
}

// CanonicalHugepagesPageSizeList normalizes and deduplicates hugepages page
// sizes while preserving the user-facing Mi/Gi suffix casing.
func CanonicalHugepagesPageSizeList(items []string) []string {
	return normalizeStringList(items, CanonicalHugepagesPageSize)
}
