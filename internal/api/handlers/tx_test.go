package handlers

import (
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestWithTxPinsReadCommittedIsolation(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "handler_tx_isolation")

	err := WithTx(t.Context(), client, func(tx *ent.Tx) error {
		return tx.ExecContext(t.Context(), `
DO $block$
BEGIN
	IF current_setting('transaction_isolation') <> 'read committed' THEN
		RAISE EXCEPTION 'unexpected transaction isolation: %', current_setting('transaction_isolation');
	END IF;
END
$block$;`)
	})
	if err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}
}
