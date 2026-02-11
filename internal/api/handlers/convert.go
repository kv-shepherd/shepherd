// Package handlers — conversion helpers for ADR-0028 omitzero boundary.
//
// With omitzero (oapi-codegen v2.5+), optional fields use value types with
// `omitzero` JSON tag instead of pointers. These helpers bridge Ent value
// types ↔ generated API types.
package handlers

import "time"

// ---- Value → Value passthrough (no pointers needed with omitzero) ----

// defaultPagination normalizes page/perPage from query params.
// With omitzero, params are int values (0 = not specified).
func defaultPagination(page, perPage int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

// ---- Pointer helpers (still needed for nillable Ent fields) ----

// timeOrZero returns the value or zero time for nillable ent fields.
func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
