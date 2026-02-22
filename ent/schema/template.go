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
		// source_type distinguishes ContainerDisk (image) from DataVolume (pvc) boot sources.
		// ADR-0036: Two boot source modes; mutually exclusive.
		field.String("source_type").
			Optional(). // "image" | "pvc"; empty means not yet configured
			Default(""),
		// image_url is the container registry URL for ContainerDisk-based templates.
		// Used when source_type == "image". Example: "quay.io/containerdisks/ubuntu:22.04"
		field.String("image_url").
			Optional(),
		// pvc_name is the DataVolume / PersistentVolumeClaim name for PVC-based templates.
		// Used when source_type == "pvc". Example: "ubuntu-22.04-base"
		field.String("pvc_name").
			Optional(),
		// cloud_init stores raw cloud-init YAML configuration (userdata).
		// Applied to VMs at boot time via cloud-init datasource.
		field.Text("cloud_init").
			Optional(),
		field.String("os_family").
			Optional(), // e.g. "linux", "windows"
		field.String("os_version").
			Optional(), // e.g. "ubuntu-22.04"
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
	}
}
