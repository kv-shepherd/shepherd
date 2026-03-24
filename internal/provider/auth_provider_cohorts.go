package provider

import "strings"

func externalCohortsFromStringValues(kind string, values []string) []ExternalCohort {
	kind = strings.TrimSpace(kind)
	if kind == "" || len(values) == 0 {
		return nil
	}

	items := make([]ExternalCohort, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cohort := ExternalCohort{
			Kind:        kind,
			Key:         value,
			DisplayName: value,
		}
		dedupeKey := cohort.Kind + ":" + cohort.Key
		if _, ok := seen[dedupeKey]; ok {
			continue
		}
		seen[dedupeKey] = struct{}{}
		items = append(items, cohort)
	}
	return items
}
