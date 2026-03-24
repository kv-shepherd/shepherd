package capabilityutil

import "strings"

// HasAllCapabilities returns true when ALL required capabilities are present in
// the cluster feature set. Matching is case-insensitive and trims whitespace.
func HasAllCapabilities(clusterFeatures, required []string) bool {
	if len(required) == 0 {
		return true
	}
	featureSet := make(map[string]struct{}, len(clusterFeatures))
	for _, f := range clusterFeatures {
		featureSet[strings.ToLower(strings.TrimSpace(f))] = struct{}{}
	}
	for _, req := range required {
		lower := strings.ToLower(strings.TrimSpace(req))
		if _, ok := featureSet[lower]; !ok {
			return false
		}
	}
	return true
}
