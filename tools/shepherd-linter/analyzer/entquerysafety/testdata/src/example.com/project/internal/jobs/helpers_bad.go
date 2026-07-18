package jobs

import _ "entgo.io/ent/dialect/sql" // want `raw Ent SQL import "entgo.io/ent/dialect/sql" is restricted`

func bad() {}
