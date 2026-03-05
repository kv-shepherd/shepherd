package provider

import (
	"context"
	"fmt"
	"testing"
)

// --------------------------------------------------------------------------
// gaFeaturesForVersion — semver edge case tests
// --------------------------------------------------------------------------

// TestGAFeaturesForVersion_SemverEdgeCases validates that the Masterminds/semver
// integration correctly handles version strings returned by KubeVirt CRs.
func TestGAFeaturesForVersion_SemverEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		version     string
		mustContain []string
		mustNotHave []string
	}{
		{
			name:        "v-prefix is handled",
			version:     "v1.7.0",
			mustContain: []string{"LiveMigration", "VMLiveUpdateFeatures"},
		},
		{
			name:        "partial version (no patch)",
			version:     "1.7",
			mustContain: []string{"LiveMigration"},
		},
		{
			name:        "pre-release is less than release",
			version:     "1.0.0-rc1",
			mustNotHave: []string{"LiveMigration"}, // 1.0.0-rc1 < 1.0.0
		},
		{
			name:        "empty string returns nil",
			version:     "",
			mustNotHave: []string{"LiveMigration"},
		},
		{
			name:        "garbage returns nil gracefully",
			version:     "not-a-version",
			mustNotHave: []string{"LiveMigration"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			features := gaFeaturesForVersion(tc.version)

			featureSet := make(map[string]bool)
			for _, f := range features {
				featureSet[f] = true
			}

			for _, want := range tc.mustContain {
				if !featureSet[want] {
					t.Errorf("version %q: expected feature %q to be present, features=%v", tc.version, want, features)
				}
			}
			for _, notWant := range tc.mustNotHave {
				if featureSet[notWant] {
					t.Errorf("version %q: feature %q should NOT be present, features=%v", tc.version, notWant, features)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// gaFeaturesForVersion tests
// --------------------------------------------------------------------------

func TestGAFeaturesForVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version     string
		mustContain []string
		mustNotHave []string
	}{
		{
			version:     "",
			mustContain: nil,
			mustNotHave: []string{"LiveMigration"},
		},
		{
			version:     "1.0.0",
			mustContain: []string{"LiveMigration"},
			mustNotHave: []string{"Snapshot"},
		},
		{
			version:     "1.1.0",
			mustContain: []string{"LiveMigration", "Snapshot", "HotplugVolumes"},
			mustNotHave: []string{"VMExport"},
		},
		{
			version:     "1.7.0", // current in go.mod
			mustContain: []string{"LiveMigration", "Snapshot", "HotplugVolumes", "VMExport", "ExpandDisks", "VMLiveUpdateFeatures"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run("version_"+tc.version, func(t *testing.T) {
			t.Parallel()
			features := gaFeaturesForVersion(tc.version)

			featureSet := make(map[string]bool)
			for _, f := range features {
				featureSet[f] = true
			}

			for _, want := range tc.mustContain {
				if !featureSet[want] {
					t.Errorf("version %s: expected feature %q to be present, features=%v", tc.version, want, features)
				}
			}
			for _, notWant := range tc.mustNotHave {
				if featureSet[notWant] {
					t.Errorf("version %s: feature %q should NOT be present, features=%v", tc.version, notWant, features)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// mergeUniqueFeatures tests
// --------------------------------------------------------------------------

func TestMergeUniqueFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b []string
		want []string
	}{
		{"both nil", nil, nil, nil},
		{"a only", []string{"A", "B"}, nil, []string{"A", "B"}},
		{"b only", nil, []string{"C"}, []string{"C"}},
		{"no overlap", []string{"A"}, []string{"B"}, []string{"A", "B"}},
		{"case dedup", []string{"LiveMigration"}, []string{"livemigration"}, []string{"LiveMigration"}},
		{"exact dedup", []string{"X", "Y"}, []string{"Y", "Z"}, []string{"X", "Y", "Z"}},
		{"empty strings filtered", []string{"A", " ", ""}, []string{"B"}, []string{"A", "B"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mergeUniqueFeatures(tc.a, tc.b)
			if len(got) != len(tc.want) {
				t.Fatalf("mergeUniqueFeatures() len=%d want=%d; got=%v want=%v", len(got), len(tc.want), got, tc.want)
			}
			for i, v := range got {
				if v != tc.want[i] {
					t.Errorf("mergeUniqueFeatures()[%d] = %q, want %q", i, v, tc.want[i])
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// ClusterCapabilities.HasFeature tests
// --------------------------------------------------------------------------

func TestClusterCapabilities_HasFeature(t *testing.T) {
	t.Parallel()

	caps := &ClusterCapabilities{
		EnabledFeatures: []string{"LiveMigration", "Snapshot", "GPU-passthrough"},
	}

	if !caps.HasFeature("LiveMigration") {
		t.Error("HasFeature(LiveMigration) = false, want true")
	}
	if !caps.HasFeature("livemigration") { // case-insensitive
		t.Error("HasFeature(livemigration) = false, want true (case-insensitive)")
	}
	if caps.HasFeature("Nonexistent") {
		t.Error("HasFeature(Nonexistent) = true, want false")
	}
	if !caps.HasAllFeatures([]string{"LiveMigration", "Snapshot"}) {
		t.Error("HasAllFeatures([LiveMigration, Snapshot]) = false, want true")
	}
	if caps.HasAllFeatures([]string{"LiveMigration", "Missing"}) {
		t.Error("HasAllFeatures([LiveMigration, Missing]) = true, want false")
	}
	if !caps.HasAllFeatures(nil) { // empty required = always true
		t.Error("HasAllFeatures(nil) = false, want true")
	}
}

// --------------------------------------------------------------------------
// HasAllCapabilities (package-level) tests
// --------------------------------------------------------------------------

func TestHasAllCapabilities(t *testing.T) {
	t.Parallel()

	clusterFeatures := []string{"LiveMigration", "GPU-passthrough", "Snapshot"}

	if !HasAllCapabilities(clusterFeatures, nil) {
		t.Error("nil required = always true")
	}
	if !HasAllCapabilities(clusterFeatures, []string{"livemigration"}) { // case-insensitive
		t.Error("case-insensitive match should succeed")
	}
	if HasAllCapabilities(clusterFeatures, []string{"SRIOV"}) {
		t.Error("SRIOV not present, should return false")
	}
	if HasAllCapabilities(nil, []string{"anything"}) {
		t.Error("empty cluster features + non-empty required = false")
	}
}

// --------------------------------------------------------------------------
// CapabilityDetector.Detect tests (using stub KubeVirtCRClient)
// --------------------------------------------------------------------------

type stubKVCRClient struct {
	gates      []string
	version    string
	err        error
	versionErr error
}

func (s *stubKVCRClient) GetFeatureGates(_ context.Context) ([]string, error) {
	return s.gates, s.err
}

func (s *stubKVCRClient) GetVersion(_ context.Context) (string, error) {
	return s.version, s.versionErr
}

type stubClusterClient struct {
	kvCR KubeVirtCRClient
}

func (s *stubClusterClient) VM() VirtualMachineClient          { return nil }
func (s *stubClusterClient) VMI() VirtualMachineInstanceClient { return nil }
func (s *stubClusterClient) SSA() DynamicSSAClient             { return nil }
func (s *stubClusterClient) KubeVirt() KubeVirtCRClient        { return s.kvCR }

func TestCapabilityDetector_Detect_MergesGAAndExplicitGates(t *testing.T) {
	t.Parallel()

	detector := NewCapabilityDetector()
	client := &stubClusterClient{
		kvCR: &stubKVCRClient{
			version: "1.7.0",                           // GetVersion() returns this
			gates:   []string{"CPUManager", "Sidecar"}, // GetFeatureGates() returns these
		},
	}

	caps, err := detector.Detect(context.Background(), client)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	// Must include GA features for v1.7.0
	if !caps.HasFeature("LiveMigration") {
		t.Error("expected GA feature LiveMigration to be present")
	}
	// Must include explicit gates
	if !caps.HasFeature("CPUManager") {
		t.Error("expected explicit gate CPUManager to be present")
	}
	if !caps.HasFeature("Sidecar") {
		t.Error("expected explicit gate Sidecar to be present")
	}
	// KubeVirtVersion must be back-filled from GetVersion()
	if caps.KubeVirtVersion != "1.7.0" {
		t.Errorf("caps.KubeVirtVersion = %q, want \"1.7.0\"", caps.KubeVirtVersion)
	}
	// No duplicates of GA features
	count := 0
	for _, f := range caps.EnabledFeatures {
		if f == "LiveMigration" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("LiveMigration appears %d times, want exactly 1", count)
	}
}

func TestCapabilityDetector_Detect_GracefulDegradationOnFeatureGatesError(t *testing.T) {
	t.Parallel()

	detector := NewCapabilityDetector()
	client := &stubClusterClient{
		kvCR: &stubKVCRClient{
			version: "1.7.0",
			err:     fmt.Errorf("permission denied on featureGates"),
		},
	}

	caps, err := detector.Detect(context.Background(), client)
	if err != nil {
		t.Fatalf("Detect() should not return error on featureGates read failure (graceful degradation), got: %v", err)
	}
	// Should still have GA features (version was fetched successfully)
	if !caps.HasFeature("LiveMigration") {
		t.Error("expected GA features even on featureGates CR read failure")
	}
	if caps.KubeVirtVersion != "1.7.0" {
		t.Errorf("caps.KubeVirtVersion = %q, want \"1.7.0\"", caps.KubeVirtVersion)
	}
}

func TestCapabilityDetector_Detect_GracefulDegradationOnVersionError(t *testing.T) {
	t.Parallel()

	detector := NewCapabilityDetector()
	client := &stubClusterClient{
		kvCR: &stubKVCRClient{
			versionErr: fmt.Errorf("permission denied on status"),
			gates:      []string{"CPUManager"}, // explicit gates still fetchable
		},
	}

	caps, err := detector.Detect(context.Background(), client)
	if err != nil {
		t.Fatalf("Detect() should not return error on version fetch failure, got: %v", err)
	}
	// GA features unavailable (version unknown), but explicit gates still merged
	if caps.HasFeature("LiveMigration") {
		t.Error("GA feature LiveMigration should NOT be present when version is unknown")
	}
	if !caps.HasFeature("CPUManager") {
		t.Error("explicit gate CPUManager should still be present even when version fetch fails")
	}
	if caps.KubeVirtVersion != "" {
		t.Errorf("caps.KubeVirtVersion = %q, want empty string on version fetch failure", caps.KubeVirtVersion)
	}
}
