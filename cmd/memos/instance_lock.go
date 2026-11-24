package main

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"slices"

	"github.com/pkg/errors"

	"github.com/usememos/memos/internal/profile"
)

// Use separate SQLite files for process locks. An immediate transaction holds
// the operating system's write lock until rollback or process exit, without
// holding any connection or transaction open on the application database.
func lockInstance(ctx context.Context, p *profile.Profile) (func(), error) {
	data, err := filepath.EvalSymlinks(p.Data)
	if err != nil {
		return nil, errors.Wrap(err, "resolve instance data directory for locking")
	}
	paths := []string{filepath.Join(data, ".cat-two.lock")}
	database, err := profile.SQLiteDatabasePath(p.DSN)
	if err != nil {
		return nil, err
	}
	if database != "" && database != ":memory:" {
		database, err = filepath.Abs(database)
		if err != nil {
			return nil, errors.Wrap(err, "resolve SQLite database path for locking")
		}
		if real, resolveErr := filepath.EvalSymlinks(database); resolveErr == nil {
			database = real
		} else if dir, resolveErr := filepath.EvalSymlinks(filepath.Dir(database)); resolveErr == nil {
			database = filepath.Join(dir, filepath.Base(database))
		}
		paths = append(paths, database+".cat-two.lock")
	}

	var releases []func()
	release := func() {
		for _, unlock := range slices.Backward(releases) {
			unlock()
		}
	}
	for _, path := range paths {
		unlock, err := lockSQLiteFile(ctx, path)
		if err != nil {
			release()
			return nil, errors.Wrapf(err, "cannot lock Cat_two instance (%s); stop any other process using its data or SQLite database", path)
		}
		releases = append(releases, unlock)
	}
	return release, nil
}

func lockSQLiteFile(ctx context.Context, path string) (func(), error) {
	uri := url.URL{Path: filepath.ToSlash(path)}
	db, err := sql.Open("sqlite", "file:"+uri.EscapedPath()+"?_txlock=immediate&_pragma=busy_timeout(0)")
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		return nil, err
	}
	return func() {
		_ = tx.Rollback()
		_ = db.Close()
	}, nil
}
