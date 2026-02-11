# River Migrations

River Queue requires its own migration tables in PostgreSQL.

## Setup

River provides a built-in migrator. Run during application startup:

```go
import "github.com/riverqueue/river/rivermigrate"

migrator, _ := rivermigrate.New(riverpgxv5.New(pool), nil)
_, _ = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
```

Or via CLI:
```bash
river migrate-up --database-url "$DATABASE_URL"
```

See: https://riverqueue.com/docs/migrations
