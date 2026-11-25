package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	storepb "github.com/usememos/memos/proto/gen/store"
)

const (
	// SQLiteApplicationID identifies databases initialized by Cat_two.
	SQLiteApplicationID = 0x43543230
	// SQLiteFileVersion matches the current schema's durable SQLite header.
	SQLiteFileVersion = 261113
	// CurrentSchemaVersion identifies the supported fresh SQLite schema.
	CurrentSchemaVersion = "26.11.13"
	// The preceding single-account schema used a separate archive flag on memos.
	archiveFileVersion   = 261112
	archiveSchemaVersion = "26.11.12"
)

//go:embed migration/sqlite/LATEST.sql
var sqliteSchema string

// ErrUnsupportedSchema indicates that the database requires a different version.
var ErrUnsupportedSchema = errors.New("unsupported Cat_two SQLite schema; use a separate empty data directory")

// InitializeSchema creates the database schema and settings on a fresh install.
// The preceding single-account schema is upgraded in place.
func (s *Store) InitializeSchema(ctx context.Context) error {
	db := s.driver.GetDB()
	if err := s.prepareSQLiteSchema(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return errors.Wrap(err, "enable SQLite write-ahead logging")
	}
	return nil
}

func (s *Store) prepareSQLiteSchema(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "begin SQLite schema check")
	}
	defer tx.Rollback()
	initialized, err := sqliteSchemaExists(ctx, tx)
	if err != nil {
		return err
	}
	if initialized {
		fileVersion, err := readSQLiteSchemaVersion(ctx, tx)
		if err != nil || fileVersion == SQLiteFileVersion {
			return err
		}
		// Only the archive flag is removed. In particular, trashed notes retain
		// their deleted_ts, and file-backed bodies keep their IDs and digests.
		if _, err := tx.ExecContext(ctx, `DROP INDEX idx_memo_owner_pinned_created;
			ALTER TABLE memo DROP COLUMN row_status;
			CREATE INDEX idx_memo_owner_pinned_created ON memo(creator_id, pinned DESC, created_ts DESC, id DESC)
				WHERE deleted_ts IS NULL;`); err != nil {
			return errors.Wrap(err, "remove memo archive state")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE system_setting
			SET value = json_set(json_remove(value, '$.schema_version'), '$.schemaVersion', ?) WHERE name = 'BASIC'`, CurrentSchemaVersion); err != nil {
			return errors.Wrap(err, "update SQLite schema setting")
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", SQLiteFileVersion)); err != nil {
			return errors.Wrap(err, "update SQLite header")
		}
		return errors.Wrap(tx.Commit(), "commit memo archive removal")
	}

	if _, err := tx.ExecContext(ctx, sqliteSchema); err != nil {
		return errors.Wrap(err, "initialize Cat_two SQLite schema")
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id = %d; PRAGMA user_version = %d", SQLiteApplicationID, SQLiteFileVersion)); err != nil {
		return errors.Wrap(err, "stamp SQLite schema")
	}
	basic := &storepb.InstanceBasicSetting{SchemaVersion: CurrentSchemaVersion}
	value, err := protojson.Marshal(basic)
	if err != nil {
		return errors.Wrap(err, "encode SQLite schema version")
	}
	if err := s.driver.UpsertInstanceSettingTx(ctx, tx, &InstanceSetting{Name: storepb.InstanceSettingKey_BASIC.String(), Value: string(value)}); err != nil {
		return errors.Wrap(err, "store SQLite schema version")
	}
	access, err := protojson.Marshal(&storepb.InstanceAccessSetting{AccessMode: storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE})
	if err != nil {
		return errors.Wrap(err, "encode instance access mode")
	}
	if err := s.driver.UpsertInstanceSettingTx(ctx, tx, &InstanceSetting{Name: storepb.InstanceSettingKey_ACCESS.String(), Value: string(access)}); err != nil {
		return errors.Wrap(err, "initialize private instance access")
	}
	return errors.Wrap(tx.Commit(), "commit SQLite schema")
}

type schemaQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func sqliteSchemaExists(ctx context.Context, db schemaQueryer) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM sqlite_schema WHERE name NOT GLOB 'sqlite_*')").Scan(&exists)
	return exists, errors.Wrap(err, "inspect SQLite schema")
}

// CheckSchemaCurrent checks the database without initializing it.
func (s *Store) CheckSchemaCurrent(ctx context.Context) error {
	db := s.driver.GetDB()
	initialized, err := sqliteSchemaExists(ctx, db)
	if err != nil {
		return err
	}
	if !initialized {
		return errors.New("the database is not initialized; start Cat_two once to create it")
	}
	fileVersion, err := readSQLiteSchemaVersion(ctx, db)
	if err != nil {
		return err
	}
	if fileVersion != SQLiteFileVersion {
		return errors.New("start Cat_two once to upgrade the SQLite schema")
	}
	return nil
}

// SupportsSQLiteFileVersion is shared with the pre-open SQLite header guard.
func SupportsSQLiteFileVersion(version uint32) bool {
	return version == SQLiteFileVersion || version == archiveFileVersion
}

func readSQLiteSchemaVersion(ctx context.Context, db schemaQueryer) (int, error) {
	var applicationID, fileVersion int
	err := db.QueryRowContext(ctx, "SELECT application_id, user_version FROM pragma_application_id(), pragma_user_version()").Scan(&applicationID, &fileVersion)
	if err != nil {
		return 0, errors.Wrap(err, "read SQLite schema marker")
	}
	if applicationID != SQLiteApplicationID || !SupportsSQLiteFileVersion(uint32(fileVersion)) {
		return 0, ErrUnsupportedSchema
	}

	var value string
	if err := db.QueryRowContext(ctx, "SELECT value FROM system_setting WHERE name = 'BASIC'").Scan(&value); err != nil {
		return 0, errors.Wrap(ErrUnsupportedSchema, "schema version setting unavailable")
	}
	setting := &storepb.InstanceBasicSetting{}
	expected := CurrentSchemaVersion
	if fileVersion == archiveFileVersion {
		expected = archiveSchemaVersion
	}
	if err := protojson.Unmarshal([]byte(value), setting); err != nil || setting.SchemaVersion != expected {
		return 0, ErrUnsupportedSchema
	}
	return fileVersion, nil
}
