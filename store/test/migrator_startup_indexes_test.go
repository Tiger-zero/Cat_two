package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationStartupIndexes(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"fresh", "upgrade"} {
		t.Run(mode, func(t *testing.T) {
			ctx := t.Context()
			ts := NewTestingStore(ctx, t)
			t.Cleanup(func() { require.NoError(t, ts.Close()) })
			db := ts.GetDriver().GetDB()
			probes := []struct {
				index string
				query string
				args  []any
			}{
				{
					index: "idx_memo_legacy_body",
					query: "SELECT id, content FROM memo WHERE id > ? AND body_digest = '' ORDER BY id LIMIT ?",
					args:  []any{0, 128},
				},
				{
					index: "idx_memo_body_digest",
					query: "SELECT DISTINCT body_digest FROM memo WHERE body_digest != ''",
				},
				{
					index: "idx_attachment_legacy_storage",
					query: "SELECT id FROM attachment WHERE id > ? AND storage_type NOT IN ('LOCAL', 'EXTERNAL', 'S3') ORDER BY id LIMIT ?",
					args:  []any{0, 64},
				},
			}
			if mode == "upgrade" {
				for _, probe := range probes {
					_, err := db.ExecContext(ctx, "DROP INDEX "+probe.index)
					require.NoError(t, err)
				}
				setSchemaVersion(ctx, t, ts, "26.11.10")
				require.NoError(t, ts.Migrate(ctx))
			}
			for _, probe := range probes {
				t.Run(probe.index, func(t *testing.T) {
					rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+probe.query, probe.args...)
					require.NoError(t, err)
					defer rows.Close()
					var plan []string
					for rows.Next() {
						var id, parent, unused int
						var detail string
						require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
						plan = append(plan, detail)
					}
					require.NoError(t, rows.Err())
					require.Contains(t, strings.Join(plan, "\n"), probe.index)
				})
			}
		})
	}
}
