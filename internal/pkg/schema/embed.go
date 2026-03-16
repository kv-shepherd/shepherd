// Package schema provides embedded KubeVirt-subset schemas and mask
// configurations for the dynamic schema API endpoint (ADR-0023).
//
// # Architecture
//
// The schema pipeline is:
//
//	KubeVirt CRD JSON Schema → (pruned subset) → embedded JSON file
//	                                         ↓
//	                       GET /schemas/{entity_type}
//	                                         ↓
//	               DynamicSchemaResponse { schema, mask, source: "embedded" }
//
// Currently all entity types are served from embedded baseline files.
// When a remote schema cache is implemented (ADR-0023 Phase 2), the handler
// SHOULD prefer the cached remote version and fall back to embedded on error.
//
// # Supported entity types
//
// - instancesize: KubeVirt v1.7.0 VirtualMachineSpec sub-schema (cpu, memory, gpu, hugepages).
//
// template and cluster are intentionally excluded:
//   - template: cloud_init is a static YAML textarea; no dynamic schema needed (master-flow Step 3).
//   - cluster:  schema not yet designed (ADR-0023 phase 2).
//
// # Adding a new entity type
//  1. Add a JSON schema file to this package directory (e.g., cluster.schema.json).
//  2. Add a JSON mask file (e.g., cluster.mask.json).
//  3. Register the entity type in SchemaFor / MaskFor switch statements.
package schema

import _ "embed"

// ─── Embedded schema files ────────────────────────────────────────────────────

// instancesize.schema.json: KubeVirt v1.7.0 VirtualMachineSpec schema, pruned
// to the spec.template subtree relevant for spec_overrides.
//
// Source: kubevirt/api v1.7.0, api/openapi-spec/swagger.json
//
//	→ definitions.v1.VirtualMachineSpec → resolved $refs inline
//	→ wrapped under root "properties.spec" to match spec_overrides path prefix.
//
// Schema version: kv-shepherd:instancesize:kubevirt-v1.7.0
// All mask paths in instancesize.mask.json must be validated against this file.
//
//go:embed instancesize.schema.json
var instancesizeSchema []byte

// instancesize.mask.json: UI projection mask for instancesize entity.
// Defines quick_fields (always visible), advanced_fields (commonly adjusted),
// and professional_fields (rare/expert settings).
//
//go:embed instancesize.mask.json
var instancesizeMask []byte

// ─── Lookup ───────────────────────────────────────────────────────────────────

// SchemaFor returns the embedded JSON schema bytes for the given entity type.
// Returns nil and false if the entity type is unknown or not yet implemented.
//
// Supported: "instancesize".
// Unknown types (including "template" and "cluster") should be rejected
// by the caller with HTTP 400.
func SchemaFor(entityType string) ([]byte, bool) {
	switch entityType {
	case "instancesize":
		return instancesizeSchema, true
	default:
		return nil, false
	}
}

// MaskFor returns the embedded JSON mask bytes for the given entity type.
// Returns nil and false if the entity type is unknown or not yet implemented.
//
// Supported: "instancesize".
// Unknown types (including "template" and "cluster") should be rejected
// by the caller with HTTP 400.
func MaskFor(entityType string) ([]byte, bool) {
	switch entityType {
	case "instancesize":
		return instancesizeMask, true
	default:
		return nil, false
	}
}
