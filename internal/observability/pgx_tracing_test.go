package observability

import "testing"

func TestSummarizeSQLUsesLowCardinalityOperationAndCollection(t *testing.T) {
	testCases := []struct {
		name           string
		sql            string
		wantOperation  string
		wantCollection string
	}{
		{
			name:           "select",
			sql:            `SELECT "tickets"."id" FROM "tickets" WHERE "tickets"."id" = $1`,
			wantOperation:  "SELECT",
			wantCollection: "tickets",
		},
		{
			name:           "insert",
			sql:            `insert into audit_logs (id, action) values ($1, $2)`,
			wantOperation:  "INSERT",
			wantCollection: "audit_logs",
		},
		{
			name:           "update",
			sql:            `UPDATE public.batch_tickets SET status = $1 WHERE id = $2`,
			wantOperation:  "UPDATE",
			wantCollection: "batch_tickets",
		},
		{
			name:           "delete",
			sql:            `DELETE FROM river_job WHERE id = $1`,
			wantOperation:  "DELETE",
			wantCollection: "river_job",
		},
		{
			name:           "malformed table token",
			sql:            `SELECT * FROM $1`,
			wantOperation:  "SELECT",
			wantCollection: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotOperation, gotCollection := summarizeSQL(tc.sql)
			if gotOperation != tc.wantOperation {
				t.Fatalf("operation = %q, want %q", gotOperation, tc.wantOperation)
			}
			if gotCollection != tc.wantCollection {
				t.Fatalf("collection = %q, want %q", gotCollection, tc.wantCollection)
			}
		})
	}
}
