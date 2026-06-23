package provider

import (
	"context"
	"strings"
	"time"

	semver "github.com/Masterminds/semver/v3"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kv-shepherd.io/shepherd/internal/provider/capabilityutil"
)

// ClusterCapabilities represents detected capabilities for a cluster (ADR-0014).
//
// Stored as JSON in Cluster.enabled_features ([]string).
// EnabledFeatures is the merged result of:
//  1. GA features guaranteed available at KubeVirtVersion (static table, no API call needed)
//  2. Explicit feature gates in kubevirt CR spec.configuration.developerConfiguration.featureGates
//
// This is the canonical structure for capability queries — prefer over raw []string from DB.
type ClusterCapabilities struct {
	KubeVirtVersion string    `json:"kubevirt_version"`
	EnabledFeatures []string  `json:"enabled_features"` // merged, lowercase-normalized keys
	DetectedAt      time.Time `json:"detected_at"`
}

// HasFeature returns true if the cluster has the specified feature enabled.
// Case-insensitive match against EnabledFeatures.
func (c *ClusterCapabilities) HasFeature(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, f := range c.EnabledFeatures {
		if strings.EqualFold(f, lower) {
			return true
		}
	}
	return false
}

// HasAllFeatures returns true when ALL required features are present.
// Used by ListCompatibleClusters (ADR-0014 Layer 3).
func (c *ClusterCapabilities) HasAllFeatures(required []string) bool {
	for _, req := range required {
		if !c.HasFeature(req) {
			return false
		}
	}
	return true
}

// CapabilityDetector detects cluster capabilities during health checks (ADR-0014).
//
// Detection strategy (2 sources merged):
//  1. GA features: inferred from KubeVirtVersion via static table — no K8s API call
//  2. Explicit featureGates: read from kubevirt CR via KubeVirtCRClient.GetFeatureGates()
//
// Called once per health check cycle per cluster (piggybacks on existing connection).
// Results are persisted to Cluster.enabled_features by lifecycle.go (P1-C).
type CapabilityDetector struct{}

// NewCapabilityDetector creates a new CapabilityDetector (stateless, safe to share).
func NewCapabilityDetector() *CapabilityDetector {
	return &CapabilityDetector{}
}

// Detect fetches live capability data from the cluster.
//
// Strategy (2 sources, merged):
//  1. GA features: inferred from Status.ObservedKubeVirtVersion via static table (no VM API calls).
//  2. Explicit featureGates: read from KubeVirt CR spec.configuration.developerConfiguration.featureGates.
//  3. Node allocatable hugepages resources: read from Nodes().List() and mapped to
//     feature keys like hugepages-2Mi / hugepages-1Gi.
//
// Both are fetched from a single successful KubeVirt CR GET (the adapter layer caches
// a successfully loaded CR object, so GetVersion() and GetFeatureGates() share one GET).
//
// Graceful degradation:
//   - If the CR GET fails (RBAC / unreachable), both version and gates degrade gracefully.
//   - Version falls back to "" → GA table returns nil.
//   - Gates fall back to nil → GA-only detection.
//   - Operator note: grant 'get kubevirts' on the 'kubevirt' namespace for full detection.
//
// Cost: exactly 1 successful KubeVirt CR GET per health check cycle per cluster
// (kubecli_adapter.go ensures the second call reuses the cached CR).
func (d *CapabilityDetector) Detect(opCtx context.Context, client KubeVirtClusterClient) (*ClusterCapabilities, error) {
	// Source 1: Observed running version → GA feature table (zero VM API calls).
	// Non-fatal: if this fails (e.g., RBAC), version stays empty and GA table returns nil.
	version, err := client.KubeVirt().GetVersion(opCtx)
	if err != nil {
		// Log-worthy but not blocking — degrade to no GA features.
		version = ""
	}
	gaFeatures := gaFeaturesForVersion(version)

	// Source 2: Explicitly configured feature gates from KubeVirt CR spec.
	// Non-fatal: if this fails (e.g., RBAC), we still return GA features.
	explicitGates, err := client.KubeVirt().GetFeatureGates(opCtx)
	if err != nil {
		// Log-worthy but not blocking — capability detection degrades gracefully.
		// Caller (lifecycle.go via health_checker.go) receives partial result and stores what we have.
		explicitGates = nil
	}

	hugepagesFeatures, err := detectHugepagesFeatures(opCtx, client.Nodes())
	if err != nil {
		// Non-fatal: a cluster may not allow node listing. Keep GA + feature gates only.
		hugepagesFeatures = nil
	}

	merged := mergeUniqueFeatures(
		mergeUniqueFeatures(gaFeatures, explicitGates),
		hugepagesFeatures,
	)

	return &ClusterCapabilities{
		KubeVirtVersion: version,
		EnabledFeatures: merged,
		DetectedAt:      time.Now(),
	}, nil
}

// HasAllCapabilities is a package-level helper for filtering clusters by feature set.
// Operates on raw []string from DB (Cluster.enabled_features), avoiding ClusterCapabilities allocation.
// Used by ListCompatibleClusters API handler (ADR-0014 Layer 3 / P2-A).
func HasAllCapabilities(clusterFeatures, required []string) bool {
	return capabilityutil.HasAllCapabilities(clusterFeatures, required)
}

// gaEntry maps a minimum KubeVirt version to the features that became GA at that version.
// Pre-parsed at package init time with semver.MustNewVersion for zero-allocation
// runtime comparisons. Panic on invalid version strings is intentional — these are
// compile-time constants maintained by us, not user input.
type gaEntry struct {
	minVersion *semver.Version
	features   []string
}

// gaTable is the GA graduation table (cumulative — each row adds features).
//
// Source: https://kubevirt.io/user-guide/cluster_admin/activating_feature_gates/
//
//	(GA = feature no longer needs to be in developerConfiguration.featureGates)
//
// Maintenance: Update this table when new features graduate to GA.
// Cadence: ~1 KubeVirt minor release per 2-3 months (check release notes).
var gaTable = []gaEntry{
	{semver.MustParse("1.0.0"), []string{"LiveMigration"}},
	{semver.MustParse("1.1.0"), []string{"Snapshot", "HotplugVolumes"}},
	{semver.MustParse("1.2.0"), []string{"VMExport", "ExpandDisks"}},
	{semver.MustParse("1.3.0"), []string{"VMLiveUpdateFeatures"}},
}

// gaFeaturesForVersion returns features that became GA (always-on by default) at a given version.
// Uses github.com/Masterminds/semver/v3 for correct semantic version comparison
// (handles pre-release, partial versions like "1.7", and v-prefix).
//
// Returns nil on empty or unparseable version string (graceful degradation).
func gaFeaturesForVersion(version string) []string {
	v := strings.TrimSpace(version)
	if v == "" {
		return nil
	}

	parsed, err := semver.NewVersion(v)
	if err != nil {
		// Unparseable version from KubeVirt CR — degrade gracefully, return no GA features.
		// This should not happen in practice (KubeVirt always reports valid semver).
		return nil
	}

	var result []string
	for _, entry := range gaTable {
		if !parsed.LessThan(entry.minVersion) { // parsed >= entry.minVersion
			result = append(result, entry.features...)
		}
	}
	return result
}

// mergeUniqueFeatures merges two []string slices, deduplicating case-insensitively.
// Preserves original casing of the first occurrence.
// Iterates each source slice directly — no intermediate temporary slice allocation.
func mergeUniqueFeatures(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	result := make([]string, 0, len(a)+len(b))
	for _, src := range [2][]string{a, b} {
		for _, v := range src {
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				continue
			}
			lower := strings.ToLower(trimmed)
			if _, ok := seen[lower]; ok {
				continue
			}
			seen[lower] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
}

func detectHugepagesFeatures(opCtx context.Context, nodes NodeClient) ([]string, error) {
	if nodes == nil {
		return nil, nil
	}
	list, err := nodes.List(opCtx, k8smetav1.ListOptions{ResourceVersion: ""})
	if err != nil {
		return nil, err
	}

	features := make([]string, 0)
	seen := make(map[string]struct{})
	for i := range list.Items {
		node := &list.Items[i]
		for resourceName, quantity := range node.Status.Allocatable {
			if quantity.Sign() <= 0 {
				continue
			}
			normalized := strings.TrimSpace(string(resourceName))
			if normalized == "" || !strings.HasPrefix(strings.ToLower(normalized), "hugepages-") {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			features = append(features, normalized)
		}
	}
	return features, nil
}
