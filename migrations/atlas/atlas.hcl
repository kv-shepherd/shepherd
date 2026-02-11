// Atlas migration configuration for KubeVirt Shepherd.
// ADR-0003: Ent ORM with Atlas for schema migrations.
//
// Usage:
//   atlas migrate diff --env shepherd
//   atlas migrate apply --env shepherd

data "composite_schema" "app" {
  // Ent schema generates the desired state.
  schema "public" {
    url = "ent://ent/schema"
  }
}

env "shepherd" {
  // Source: composite Ent schema.
  src = data.composite_schema.app.url

  // Target: PostgreSQL database.
  url = getenv("DATABASE_URL")

  // Migration directory.
  migration {
    dir = "file://migrations/atlas"
  }

  // Dev database for diffing (ephemeral).
  dev = "docker://postgres/18/dev?search_path=public"
}

env "ci" {
  src = data.composite_schema.app.url
  url = getenv("DATABASE_URL")
  migration {
    dir = "file://migrations/atlas"
  }
  dev = "docker://postgres/18/dev?search_path=public"
}
