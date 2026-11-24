package test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
	"github.com/usememos/memos/store/db"
)

func TestInitializeSQLiteSchemaAndRestart(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	p := getTestingProfile(t)
	p.Personal = true
	p.InstanceURL = "https://notes.example.test"
	open := func() *store.Store {
		t.Helper()
		driver, err := db.NewDBDriver(p)
		require.NoError(t, err)
		st := store.New(driver, p)
		t.Cleanup(func() { _ = st.Close() })
		return st
	}
	first := open()
	require.ErrorContains(t, first.CheckSchemaCurrent(ctx), "not initialized")
	require.NoError(t, first.InitializeSchema(ctx))
	require.NoError(t, first.CheckSchemaCurrent(ctx))
	basic, err := first.GetInstanceBasicSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, store.CurrentSchemaVersion, basic.GetSchemaVersion())
	access, err := first.GetInstanceAccessSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE, access.GetAccessMode())

	owner, err := createTestingHostUser(ctx, first)
	require.NoError(t, err)
	created, err := first.CreateMemo(ctx, &store.Memo{UID: "schema-restart", CreatorID: owner.ID, Content: "kept after restart", Visibility: store.Private})
	require.NoError(t, err)
	require.NotEmpty(t, created.BodyDigest)
	require.NoError(t, first.Close())

	second := open()
	require.NoError(t, second.InitializeSchema(ctx))
	require.NoError(t, second.CheckSchemaCurrent(ctx))
	memo, err := second.GetMemo(ctx, &store.FindMemo{ID: &created.ID})
	require.NoError(t, err)
	require.Equal(t, created.Content, memo.Content)
	require.Equal(t, created.BodyDigest, memo.BodyDigest)
}

func TestSQLiteRejectsOlderDatabaseWithoutWriting(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	p := getTestingProfile(t)
	legacy, err := sql.Open("sqlite", p.DSN)
	require.NoError(t, err)
	_, err = legacy.ExecContext(ctx, `
		CREATE TABLE system_setting (name TEXT NOT NULL, value TEXT NOT NULL);
		INSERT INTO system_setting VALUES ('BASIC', '{"schemaVersion":"26.11.11"}');
		CREATE TABLE memo (id INTEGER PRIMARY KEY, content TEXT);
		INSERT INTO memo (content) VALUES ('previous note');
	`)
	require.NoError(t, err)
	require.NoError(t, legacy.Close())

	before, err := os.ReadFile(p.DSN)
	require.NoError(t, err)
	noteFile := filepath.Join(p.Data, "preserve.md")
	require.NoError(t, os.WriteFile(noteFile, []byte("old note"), 0o600))
	driver, err := db.NewDBDriver(p)
	require.ErrorIs(t, err, store.ErrUnsupportedSchema)
	require.Nil(t, driver)

	after, err := os.ReadFile(p.DSN)
	require.NoError(t, err)
	require.Equal(t, before, after)
	content, err := os.ReadFile(noteFile)
	require.NoError(t, err)
	require.Equal(t, "old note", string(content))
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		require.NoFileExists(t, p.DSN+suffix)
	}
}

func TestSQLiteRejectsInconsistentSchemaStamp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	_, err := st.GetDriver().GetDB().ExecContext(ctx,
		"UPDATE system_setting SET value = ? WHERE name = 'BASIC'", `{"schemaVersion":"26.11.11"}`)
	require.NoError(t, err)
	require.ErrorIs(t, st.CheckSchemaCurrent(ctx), store.ErrUnsupportedSchema)
	require.ErrorIs(t, st.InitializeSchema(ctx), store.ErrUnsupportedSchema)
	var mark string
	err = st.GetDriver().GetDB().QueryRowContext(ctx, "SELECT value FROM system_setting WHERE name = 'BASIC'").Scan(&mark)
	require.NoError(t, err)
	require.JSONEq(t, `{"schemaVersion":"26.11.11"}`, mark)
}

func TestSQLiteRejectsMissingDatabaseWithRecoveryJournal(t *testing.T) {
	t.Parallel()
	p := getTestingProfile(t)
	journal := p.DSN + "-wal"
	original := []byte("existing recovery journal")
	require.NoError(t, os.WriteFile(journal, original, 0o600))

	driver, err := db.NewDBDriver(p)
	require.ErrorIs(t, err, store.ErrUnsupportedSchema)
	require.Nil(t, driver)
	require.NoFileExists(t, p.DSN)
	contents, err := os.ReadFile(journal)
	require.NoError(t, err)
	require.Equal(t, original, contents)
}
