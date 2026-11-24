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
	SQLiteFileVersion = 261112
	// CurrentSchemaVersion identifies the supported fresh SQLite schema.
	CurrentSchemaVersion = "26.11.12"
)

//go:embed migration/sqlite/LATEST.sql
var sqliteSchema string

// ErrUnsupportedSchema indicates that the database requires a different version.
var ErrUnsupportedSchema = errors.New("unsupported Cat_two SQLite schema; use a separate empty data directory")

// InitializeSchema creates the database schema and settings on a fresh install.
// Existing databases must match this version exactly.
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
		return checkSchemaCurrent(ctx, tx)
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
	return checkSchemaCurrent(ctx, db)
}

func checkSchemaCurrent(ctx context.Context, db schemaQueryer) error {
	var applicationID, fileVersion int
	err := db.QueryRowContext(ctx, "SELECT application_id, user_version FROM pragma_application_id(), pragma_user_version()").Scan(&applicationID, &fileVersion)
	if err != nil {
		return errors.Wrap(err, "read SQLite schema marker")
	}
	if applicationID != SQLiteApplicationID || fileVersion != SQLiteFileVersion {
		return ErrUnsupportedSchema
	}

	var value string
	if err := db.QueryRowContext(ctx, "SELECT value FROM system_setting WHERE name = 'BASIC'").Scan(&value); err != nil {
		return errors.Wrap(ErrUnsupportedSchema, "schema version setting unavailable")
	}
	setting := &storepb.InstanceBasicSetting{}
	if err := protojson.Unmarshal([]byte(value), setting); err != nil || setting.SchemaVersion != CurrentSchemaVersion {
		return ErrUnsupportedSchema
	}
	return nil
}
