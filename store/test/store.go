package test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/version"
	"github.com/usememos/memos/store"
	"github.com/usememos/memos/store/db"
)

// NewTestingStore creates a fresh SQLite database in a temporary directory.
func NewTestingStore(ctx context.Context, t *testing.T) *store.Store {
	p := getTestingProfile(t)
	dbDriver, err := db.NewDBDriver(p)
	if err != nil {
		t.Fatalf("failed to create db driver: %v", err)
	}

	store := store.New(dbDriver, p)
	if err := store.InitializeSchema(ctx); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}
	return store
}

func getTestingProfile(t *testing.T) *profile.Profile {
	t.Helper()
	dir := t.TempDir()
	return &profile.Profile{
		Data:    dir,
		DSN:     filepath.Join(dir, "memos_prod.db"),
		Driver:  profile.SQLiteDriver,
		Version: version.GetCurrentVersion(),
	}
}
