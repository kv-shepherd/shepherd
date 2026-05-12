package service

import (
	"fmt"
	"strings"
)

const (
	SystemLabelOSAny     = "os:any"
	SystemLabelOSLinux   = "os:linux"
	SystemLabelOSWindows = "os:windows"
)

var platformSystemLabels = []string{
	SystemLabelOSAny,
	SystemLabelOSLinux,
	SystemLabelOSWindows,
}

var platformSystemLabelSet = map[string]struct{}{
	SystemLabelOSAny:     {},
	SystemLabelOSLinux:   {},
	SystemLabelOSWindows: {},
}

// PlatformSystemLabels returns the current platform-defined compatibility labels.
// Users can select these labels, but cannot create arbitrary compatibility tags.
func PlatformSystemLabels() []string {
	out := make([]string, len(platformSystemLabels))
	copy(out, platformSystemLabels)
	return out
}

// NormalizeTemplateSystemLabels validates and normalizes template compatibility labels.
// Templates express requirements, so only one concrete OS label is allowed.
func NormalizeTemplateSystemLabels(labels []string) ([]string, error) {
	return normalizeSystemLabels(labels, false)
}

// NormalizeInstanceSizeSystemLabels validates and normalizes instance-size compatibility
// labels. Instance sizes express support, so multiple concrete OS labels are allowed.
func NormalizeInstanceSizeSystemLabels(labels []string) ([]string, error) {
	return normalizeSystemLabels(labels, true)
}

// NormalizeSystemLabelsForRead keeps legacy rows compatible: absent labels mean
// the generic OS label and do not restrict template/size pairing.
func NormalizeSystemLabelsForRead(labels []string) []string {
	normalized, err := normalizeSystemLabels(labels, true)
	if err != nil {
		return []string{SystemLabelOSAny}
	}
	return normalized
}

// TemplateInstanceSizeCompatible reports whether a template requirement set can
// use an instance-size support set. For each label group, "group:any" is a wildcard.
func TemplateInstanceSizeCompatible(templateLabels, instanceSizeLabels []string) bool {
	templateGroups := systemLabelsByGroup(NormalizeSystemLabelsForRead(templateLabels))
	sizeGroups := systemLabelsByGroup(NormalizeSystemLabelsForRead(instanceSizeLabels))

	for group, templateValues := range templateGroups {
		if hasLabelValue(templateValues, "any") {
			continue
		}
		sizeValues := sizeGroups[group]
		if len(sizeValues) == 0 || hasLabelValue(sizeValues, "any") {
			continue
		}
		if !labelValuesIntersect(templateValues, sizeValues) {
			return false
		}
	}
	return true
}

func normalizeSystemLabels(labels []string, allowMultipleConcreteOS bool) ([]string, error) {
	normalizedSet := make(map[string]struct{}, len(labels))
	for _, raw := range labels {
		label := strings.ToLower(strings.TrimSpace(raw))
		if label == "" {
			continue
		}
		if _, ok := platformSystemLabelSet[label]; !ok {
			return nil, fmt.Errorf("unsupported system label %q", raw)
		}
		normalizedSet[label] = struct{}{}
	}

	if len(normalizedSet) == 0 {
		return []string{SystemLabelOSAny}, nil
	}

	concreteOSCount := 0
	for label := range normalizedSet {
		if strings.HasPrefix(label, "os:") && label != SystemLabelOSAny {
			concreteOSCount++
		}
	}
	if _, hasAny := normalizedSet[SystemLabelOSAny]; hasAny && concreteOSCount > 0 {
		return nil, fmt.Errorf("%s cannot be combined with concrete OS labels", SystemLabelOSAny)
	}
	if !allowMultipleConcreteOS && concreteOSCount > 1 {
		return nil, fmt.Errorf("template can require only one concrete OS label")
	}

	ordered := make([]string, 0, len(normalizedSet))
	for _, label := range platformSystemLabels {
		if _, ok := normalizedSet[label]; ok {
			ordered = append(ordered, label)
		}
	}
	return ordered, nil
}

func systemLabelsByGroup(labels []string) map[string][]string {
	groups := make(map[string][]string)
	for _, label := range labels {
		group, value, ok := strings.Cut(label, ":")
		if !ok || group == "" || value == "" {
			continue
		}
		groups[group] = append(groups[group], value)
	}
	return groups
}

func hasLabelValue(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func labelValuesIntersect(left, right []string) bool {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	for _, value := range left {
		if _, ok := rightSet[value]; ok {
			return true
		}
	}
	return false
}
