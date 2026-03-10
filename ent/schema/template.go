package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Template holds the schema definition for the Template entity.
// ADR-0018: Templates stored in PostgreSQL, not as YAML files.
// ADR-0036: Template contains software-baseline only (source + cloud-init).
type Template struct {
	ent.Schema
}

// Mixin of the Template.
func (Template) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the Template.
func (Template) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.String("display_name").
			Optional(),
		field.String("description").
			Optional(),
		// source_type identifies the VM boot source provisioning mode.
		// Canonical values: "containerdisk", "cdi_image_import", "cdi_pvc_clone".
		// Empty means not yet configured.
		// ADR-0036: Boot source modes are mutually exclusive.
		field.String("source_type").
			Optional().
			Default(""),
		// image_url is the container registry URL for containerdisk or CDI image import.
		// Used when source_type is "containerdisk" or "cdi_image_import".
		// Example: "quay.io/containerdisks/ubuntu:22.04"
		field.String("image_url").
			Optional(),
		// pvc_name is the source PVC name for CDI clone-based templates.
		// Used when source_type == "cdi_pvc_clone". Example: "ubuntu-22.04-golden"
		field.String("pvc_name").
			Optional(),
		// pvc_namespace is the Kubernetes namespace of the source PVC for CDI clone.
		// Used when source_type == "cdi_pvc_clone". Required together with pvc_name.
		// The VM creation worker renders this into dataVolumeTemplates.spec.source.pvc.namespace.
		field.String("pvc_namespace").
			Optional(),
		// cloud_init stores raw cloud-init YAML configuration (userdata).
		// Applied to VMs at boot time via cloud-init datasource.
		field.Text("cloud_init").
			Optional(),
		field.String("os_family").
			Optional(), // e.g. "linux", "windows"
		field.String("os_version").
			Optional(), // e.g. "ubuntu-22.04"
		field.Enum("catalog_scope").
			Values("unclassified", "test", "prod", "all").
			Default("unclassified").
			Comment("Catalog visibility scope only. Not scheduling environment."),
		field.Bool("enabled").
			Default(true),
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the Template.
func (Template) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("enabled"),
		index.Fields("source_type"),
		index.Fields("catalog_scope"),
	}
}
