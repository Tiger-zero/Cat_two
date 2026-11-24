package sqlite

import (
	"github.com/pkg/errors"
	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// IsRetryableTransactionError reports whether err is a transient SQLite locking failure.
func (*DB) IsRetryableTransactionError(err error) bool {
	var sqliteErr *msqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code() & 0xff
	return code == sqlite3.SQLITE_BUSY || code == sqlite3.SQLITE_LOCKED
}
