package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

// rejectStoreDelete injects a real database failure in an isolated test database.
// Table names are fixed by the tests, never supplied by application input.
func rejectStoreDelete(t *testing.T, ts *store.Store, table string) func() {
	t.Helper()
	statement := fmt.Sprintf(`CREATE TRIGGER reject_store_delete BEFORE DELETE ON %s
		BEGIN SELECT RAISE(ABORT, 'store test delete failure'); END`, table)
	_, err := ts.GetDriver().GetDB().ExecContext(context.Background(), statement)
	require.NoError(t, err)
	return func() {
		t.Helper()
		_, err := ts.GetDriver().GetDB().ExecContext(context.Background(), "DROP TRIGGER reject_store_delete")
		require.NoError(t, err)
	}
}
