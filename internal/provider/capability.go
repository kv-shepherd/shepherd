package provider

import (
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterCapabilities represents detected capabilities for a cluster (ADR-0014).
type ClusterCapabilities struct {
	KubeVirtVersion string            `json:"kubevirt_version"`
	Features        map[string]bool   `json:"features"`
	Hardware        map[string]bool   `json:"hardware_capabilities"`
}

// CapabilityDetector detects cluster capabilities during health checks.
type CapabilityDetector struct{}

// NewCapabilityDetector creates a new CapabilityDetector.
func NewCapabilityDetector() *CapabilityDetector {
	return &CapabilityDetector{}
}

// DetectCapabilities detects capabilities for a cluster.
// Called during health check cycle (piggybacks on existing connection).
func (d *CapabilityDetector) DetectCapabilities(clusterName string, health *ClusterHealth) *ClusterCapabilities {
	caps := &ClusterCapabilities{
		KubeVirtVersion: health.KubeVirtVersion,
		Features:        make(map[string]bool),
		Hardware:        make(map[string]bool),
	}

	// Static GA table: features that are GA by version
	if health.KubeVirtVersion != "" {
		applyGAFeatures(caps, health.KubeVirtVersion)
	}

	return caps
}

// HasCapability checks if a cluster has a specific capability.
func (caps *ClusterCapabilities) HasCapability(name string) bool {
	if v, ok := caps.Features[name]; ok {
		return v
	}
	if v, ok := caps.Hardware[name]; ok {
		return v
	}
	return false
}

// HasAllCapabilities checks if a cluster has all required capabilities.
// Used by FilterCompatibleClusters (ADR-0018: requirements from InstanceSize).
func (caps *ClusterCapabilities) HasAllCapabilities(required []string) bool {
	for _, req := range required {
		if !caps.HasCapability(req) {
			return false
		}
	}
	return true
}

// applyGAFeatures applies features that became GA by a specific KubeVirt version.
func applyGAFeatures(caps *ClusterCapabilities, version string) {
	// KubeVirt GA features by version (static table)
	// These features are always available in the specified version and later
	gaFeatures := map[string]string{
		"LiveMigration":        "1.0.0",
		"Snapshot":             "1.1.0",
		"HotplugVolumes":       "1.1.0",
		"VMExport":             "1.2.0",
		"ExpandDisks":          "1.2.0",
		"VMLiveUpdateFeatures": "1.3.0",
	}

	for feature, minVersion := range gaFeatures {
		if versionGTE(version, minVersion) {
			caps.Features[feature] = true
		}
	}
}

// versionGTE returns true if version >= minVersion (simplified semver comparison).
func versionGTE(version, minVersion string) bool {
	// Simplified: compare version strings lexicographically
	// Production code should use semver library
	return version >= minVersion
}

// defaultListOpts returns default list options for health check queries.
func defaultListOpts() k8smetav1.ListOptions {
	return k8smetav1.ListOptions{Limit: 1}
}
