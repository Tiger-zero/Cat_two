package sqlite

import (
	"context"

	"github.com/usememos/memos/store"
)

func (d *DB) GetMemoBodyNamespace(ctx context.Context) (string, error) {
	var namespace string
	err := d.db.QueryRowContext(ctx, `SELECT namespace FROM memo_body_namespace WHERE id = 1`).Scan(&namespace)
	return namespace, err
}

func (d *DB) ListMemoBodyDigests(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT DISTINCT body_digest FROM memo WHERE body_digest != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var digests []string
	for rows.Next() {
		var digest string
		if err := rows.Scan(&digest); err != nil {
			return nil, err
		}
		digests = append(digests, digest)
	}
	return digests, rows.Err()
}

func (d *DB) ListLegacyMemoBodies(ctx context.Context, afterID int32, limit int) ([]store.LegacyMemoBody, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id, content FROM memo WHERE id > ? AND body_digest = '' ORDER BY id LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bodies []store.LegacyMemoBody
	for rows.Next() {
		var body store.LegacyMemoBody
		if err := rows.Scan(&body.ID, &body.Content); err != nil {
			return nil, err
		}
		bodies = append(bodies, body)
	}
	return bodies, rows.Err()
}

func (d *DB) ListMemoBodyReferences(ctx context.Context, afterID int32, limit int) ([]store.MemoBodyReference, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id, body_digest FROM memo WHERE id > ? AND body_digest != '' ORDER BY id LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bodies []store.MemoBodyReference
	for rows.Next() {
		var body store.MemoBodyReference
		if err := rows.Scan(&body.ID, &body.Digest); err != nil {
			return nil, err
		}
		bodies = append(bodies, body)
	}
	return bodies, rows.Err()
}

func (d *DB) GetMemoBodyProjection(ctx context.Context, id int32, digest string) (string, error) {
	var content string
	err := d.db.QueryRowContext(ctx, `SELECT content FROM memo WHERE id = ? AND body_digest = ?`, id, digest).Scan(&content)
	return content, err
}

func (d *DB) SetMemoBodyDigest(ctx context.Context, id int32, content, digest string) (bool, error) {
	result, err := d.db.ExecContext(ctx, `UPDATE memo SET body_digest = ? WHERE id = ? AND body_digest = '' AND content = ?`, digest, id, content)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	return changed == 1, err
}
