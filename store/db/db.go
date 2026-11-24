package db

import (
	"github.com/pkg/errors"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/store"
	"github.com/usememos/memos/store/db/sqlite"
)

// NewDBDriver opens the SQLite database configured in profile.
func NewDBDriver(p *profile.Profile) (store.Driver, error) {
	if p.Driver != "" && p.Driver != profile.SQLiteDriver {
		return nil, errors.Errorf("unsupported database driver %q: use sqlite", p.Driver)
	}
	driver, err := sqlite.NewDB(p)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open SQLite database")
	}
	return driver, nil
}
