package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestValidateSQLiteMemoCreatePreservesQueryErrors(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`
		CREATE TABLE user (id INTEGER PRIMARY KEY, row_status TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'USER');
		INSERT INTO user (id, row_status) VALUES (1, 'NORMAL'), (2, 'ARCHIVED');
	`)
	require.NoError(t, err)
	require.NoError(t, validateSQLiteMemoCreate(context.Background(), db, &store.Memo{CreatorID: 1}))

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	err = validateSQLiteMemoCreate(canceledContext, db, &store.Memo{CreatorID: 1})
	require.ErrorIs(t, err, context.Canceled)

	err = validateSQLiteMemoCreate(context.Background(), db, &store.Memo{CreatorID: 99})
	require.ErrorIs(t, err, store.ErrMemoPermissionDenied)
	err = validateSQLiteMemoCreate(context.Background(), db, &store.Memo{CreatorID: 2})
	require.ErrorIs(t, err, store.ErrMemoPermissionDenied)
}
