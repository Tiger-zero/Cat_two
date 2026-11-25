package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"

	// Note: modernc.org/sqlite driver is imported in functions.go where
	// RegisterScalarFunction is used. No blank import needed here.

	profilepkg "github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/store"
)

type DB struct {
	db      *sql.DB
	profile *profilepkg.Profile
}

// NewDB opens a database specified by its database driver name and a
// driver-specific data source name, usually consisting of at least a
// database name and connection information.
func NewDB(profile *profilepkg.Profile) (store.Driver, error) {
	// Ensure a DSN is set before attempting to open the database.
	if profile.DSN == "" {
		return nil, errors.New("dsn required")
	}

	if err := ensureUnicodeLowerRegistered(); err != nil {
		return nil, errors.Wrap(err, "failed to register sqlite unicode lower function")
	}

	if err := ensureRegexpRegistered(); err != nil {
		return nil, errors.Wrap(err, "failed to register sqlite regexp function")
	}

	path, err := profilepkg.SQLiteDatabasePath(profile.DSN)
	if err != nil {
		return nil, err
	}
	if err := checkSQLiteFileVersion(path); err != nil {
		return nil, err
	}

	// The schema check runs before enabling WAL. Transactions take their write
	// lock up front so read-before-write operations honor busy_timeout.
	separator := "?"
	if strings.Contains(profile.DSN, "?") {
		separator = "&"
	}
	sqliteDB, err := sql.Open("sqlite", profile.DSN+separator+"_pragma=foreign_keys(0)&_pragma=busy_timeout(10000)&_pragma=mmap_size(0)&_txlock=immediate")
	if err != nil {
		return nil, errors.Wrap(err, "open Cat_two SQLite database")
	}

	driver := DB{db: sqliteDB, profile: profile}

	return &driver, nil
}

func (d *DB) GetDB() *sql.DB {
	return d.db
}

func (d *DB) Close() error {
	return d.db.Close()
}

// checkSQLiteFileVersion rejects legacy files before SQLite has a chance to
// recover their journals or change their persistent journal mode.
func checkSQLiteFileVersion(path string) error {
	if path == ":memory:" {
		return nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return requireNoSQLiteJournal(path)
	}
	if err != nil {
		return errors.Wrap(err, "read SQLite database header")
	}
	defer file.Close()

	var header [72]byte
	n, err := io.ReadFull(file, header[:])
	if n == 0 && errors.Is(err, io.EOF) {
		return requireNoSQLiteJournal(path)
	}
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.Wrap(err, "read SQLite database header")
	}
	if n < len(header) || string(header[:16]) != "SQLite format 3\x00" ||
		!store.SupportsSQLiteFileVersion(binary.BigEndian.Uint32(header[60:64])) ||
		binary.BigEndian.Uint32(header[68:72]) != store.SQLiteApplicationID {
		return store.ErrUnsupportedSchema
	}
	return nil
}

func requireNoSQLiteJournal(path string) error {
	for _, suffix := range []string{"-wal", "-journal"} {
		info, err := os.Stat(path + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return errors.Wrap(err, "inspect SQLite journal")
		}
		if info.Size() != 0 {
			return store.ErrUnsupportedSchema
		}
	}
	return nil
}

// GetDatabaseSize returns the database size in bytes, or -1 if unavailable.
func (d *DB) GetDatabaseSize(ctx context.Context) (int64, error) {
	var pageCount, pageSize int64
	if err := d.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
		return -1, errors.Wrap(err, "failed to read page_count")
	}
	if err := d.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return -1, errors.Wrap(err, "failed to read page_size")
	}
	return pageCount * pageSize, nil
}
