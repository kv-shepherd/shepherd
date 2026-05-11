package handlers

import (
	_ "entgo.io/ent/dialect/sql"         // want `raw Ent SQL import "entgo.io/ent/dialect/sql" is restricted`
	_ "entgo.io/ent/dialect/sql/sqljson" // want `raw Ent SQL import "entgo.io/ent/dialect/sql/sqljson" is restricted`
)

func bad() {}
