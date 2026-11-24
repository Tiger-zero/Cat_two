// Package legacyspace seeds historical workspace records in SQLite test databases.
package legacyspace

import (
	"context"
	"database/sql"
)

// Seed inserts a legacy workspace and its owner membership for compatibility tests.
func Seed(ctx context.Context, db *sql.DB, uid, title string, ownerID int32) (int32, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var id int32
	if err := tx.QueryRowContext(ctx, `INSERT INTO space (uid, title) VALUES (?, ?) RETURNING id`, uid, title).Scan(&id); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO space_member (space_id, user_id, status, role) VALUES (?, ?, 'ACTIVE', 'ADMIN')`, id, ownerID); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// AddMember inserts an active membership into a seeded legacy workspace.
func AddMember(ctx context.Context, db *sql.DB, spaceID, userID int32, role string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO space_member (space_id, user_id, status, role) VALUES (?, ?, 'ACTIVE', ?)`, spaceID, userID, role)
	return err
}

// RemoveMember removes an active membership from a seeded legacy workspace.
func RemoveMember(ctx context.Context, db *sql.DB, spaceID, userID int32) error {
	_, err := db.ExecContext(ctx, `DELETE FROM space_member WHERE space_id = ? AND user_id = ? AND status = 'ACTIVE'`, spaceID, userID)
	return err
}

// PlaceMemo recreates historical placement and visibility for an existing memo.
func PlaceMemo(ctx context.Context, db *sql.DB, memoID, spaceID int32, visibility string) error {
	_, err := db.ExecContext(ctx, `UPDATE memo SET space_id = ?, visibility = ? WHERE id = ?`, spaceID, visibility, memoID)
	return err
}
