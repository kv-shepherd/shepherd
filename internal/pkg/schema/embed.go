// Package schema provides embedded KubeVirt-subset schemas and mask
// configurations for the dynamic schema API endpoint (ADR-0023).
//
// # Architecture
//
// The schema pipeline is:
//
//	KubeVirt CRD JSON Schema → (pruned subset) → versioned embedded JSON files
//	                                                ↓
//	                                  manifest.json selects current baseline
//	                                                ↓
//	                         GET /schemas/{entity_type} → DynamicSchemaResponse
//
// Currently all entity types are served from embedded baseline files selected by
// manifest.json. When a remote schema cache is implemented (ADR-0023 Phase 2),
// handlers SHOULD prefer the cached remote version and fall back to the current
// embedded baseline on error.
package schema

import (
	"embed"
	"encoding/json"
	"sync"
)

//go:embed manifest.json versions/*/*.json
var embeddedFiles embed.FS

type embeddedSchemaManifest struct {
	Entities map[string]embeddedSchemaEntity `json:"entities"`
}

type embeddedSchemaEntity struct {
	CurrentVersion string                           `json:"current_version"`
	Versions       map[string]embeddedSchemaVersion `json:"versions"`
}

type embeddedSchemaVersion struct {
	KubeVirtVersion string `json:"kubevirt_version"`
	SchemaPath      string `json:"schema_path"`
	MaskPath        string `json:"mask_path"`
}

// EmbeddedVersionInfo describes one embedded schema baseline.
type EmbeddedVersionInfo struct {
	Key             string
	KubeVirtVersion string
}

var (
	manifestOnce sync.Once
	manifestData embeddedSchemaManifest
	errManifest  error
)

func loadManifest() error {
	manifestOnce.Do(func() {
		raw, err := embeddedFiles.ReadFile("manifest.json")
		if err != nil {
			errManifest = err
			return
		}
		if err := json.Unmarshal(raw, &manifestData); err != nil {
			errManifest = err
			return
		}
	})
	return errManifest
}

func entityConfig(entityType string) (embeddedSchemaEntity, bool) {
	if err := loadManifest(); err != nil {
		return embeddedSchemaEntity{}, false
	}
	cfg, ok := manifestData.Entities[entityType]
	return cfg, ok
}

func versionConfig(entityType, versionKey string) (embeddedSchemaVersion, bool) {
	cfg, ok := entityConfig(entityType)
	if !ok {
		return embeddedSchemaVersion{}, false
	}
	version, ok := cfg.Versions[versionKey]
	return version, ok
}

func readEmbeddedJSON(path string) ([]byte, bool) {
	if path == "" {
		return nil, false
	}
	raw, err := embeddedFiles.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// CurrentVersionKeyFor returns the active embedded version key for an entity.
func CurrentVersionKeyFor(entityType string) (string, bool) {
	cfg, ok := entityConfig(entityType)
	if !ok || cfg.CurrentVersion == "" {
		return "", false
	}
	return cfg.CurrentVersion, true
}

// SchemaVersionFor returns the current KubeVirt semver for an entity.
func SchemaVersionFor(entityType string) (string, bool) {
	versionKey, ok := CurrentVersionKeyFor(entityType)
	if !ok {
		return "", false
	}
	return SchemaVersionForVersion(entityType, versionKey)
}

// SchemaVersionForVersion returns the KubeVirt semver for a specific embedded version key.
func SchemaVersionForVersion(entityType, versionKey string) (string, bool) {
	version, ok := versionConfig(entityType, versionKey)
	if !ok || version.KubeVirtVersion == "" {
		return "", false
	}
	return version.KubeVirtVersion, true
}

// VersionKeyForKubeVirtVersion resolves a KubeVirt semver (for example "1.8.0")
// to the embedded version key (for example "kubevirt-v1.8.0").
func VersionKeyForKubeVirtVersion(entityType, kubeVirtVersion string) (string, bool) {
	cfg, ok := entityConfig(entityType)
	if !ok {
		return "", false
	}
	for key, version := range cfg.Versions {
		if version.KubeVirtVersion == kubeVirtVersion {
			return key, true
		}
	}
	return "", false
}

// AvailableVersions returns the embedded schema versions for an entity.
func AvailableVersions(entityType string) ([]EmbeddedVersionInfo, bool) {
	cfg, ok := entityConfig(entityType)
	if !ok {
		return nil, false
	}
	versions := make([]EmbeddedVersionInfo, 0, len(cfg.Versions))
	for key, version := range cfg.Versions {
		versions = append(versions, EmbeddedVersionInfo{
			Key:             key,
			KubeVirtVersion: version.KubeVirtVersion,
		})
	}
	sortEmbeddedVersions(versions)
	return versions, true
}

// AvailableSchemaVersions returns sorted KubeVirt semvers for one entity.
func AvailableSchemaVersions(entityType string) ([]string, bool) {
	versions, ok := AvailableVersions(entityType)
	if !ok {
		return nil, false
	}
	items := make([]string, 0, len(versions))
	for _, version := range versions {
		if version.KubeVirtVersion == "" {
			continue
		}
		items = append(items, version.KubeVirtVersion)
	}
	return items, true
}

// SchemaFor returns the embedded JSON schema bytes for the current entity baseline.
func SchemaFor(entityType string) ([]byte, bool) {
	versionKey, ok := CurrentVersionKeyFor(entityType)
	if !ok {
		return nil, false
	}
	return SchemaForVersion(entityType, versionKey)
}

// SchemaForVersion returns the embedded JSON schema bytes for a specific version key.
func SchemaForVersion(entityType, versionKey string) ([]byte, bool) {
	version, ok := versionConfig(entityType, versionKey)
	if !ok {
		return nil, false
	}
	return readEmbeddedJSON(version.SchemaPath)
}

// MaskFor returns the embedded JSON mask bytes for the current entity baseline.
func MaskFor(entityType string) ([]byte, bool) {
	versionKey, ok := CurrentVersionKeyFor(entityType)
	if !ok {
		return nil, false
	}
	return MaskForVersion(entityType, versionKey)
}

// MaskForVersion returns the embedded JSON mask bytes for a specific version key.
func MaskForVersion(entityType, versionKey string) ([]byte, bool) {
	version, ok := versionConfig(entityType, versionKey)
	if !ok {
		return nil, false
	}
	return readEmbeddedJSON(version.MaskPath)
}
