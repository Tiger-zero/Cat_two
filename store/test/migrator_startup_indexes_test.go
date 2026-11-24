package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartupMemoDigestUsesCoveringIndex(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	rows, err := ts.GetDriver().GetDB().QueryContext(ctx,
		"EXPLAIN QUERY PLAN SELECT DISTINCT body_digest FROM memo WHERE body_digest != ''")
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
	require.Contains(t, strings.Join(plan, "\n"), "COVERING INDEX idx_memo_body_digest")
}
