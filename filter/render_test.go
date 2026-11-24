package filter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Callers append their own predicates after the filter fragment and bind
// positionally, so an argument left behind by a folded-away operand would bind
// to the caller's next placeholder. Folded operands must take their arguments
// with them.
func TestRenderFoldedOperandsDropTheirArgs(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	tests := []struct {
		filter string
		sql    string // "" for trivially true, "1 = 0" for unsatisfiable, "*" for any non-constant fragment
		args   []any
	}{
		{filter: `creator_id == 1 || true`, sql: "", args: []any{}},
		{filter: `true || creator_id == 1`, sql: "", args: []any{}},
		{filter: `creator_id == 1 && false`, sql: "1 = 0", args: []any{}},
		{filter: `false && creator_id == 1`, sql: "1 = 0", args: []any{}},
		{filter: `!(creator_id == 1 || true)`, sql: "1 = 0", args: []any{}},
		{filter: `!(creator_id == 1 && false)`, sql: "", args: []any{}},
		{filter: `(creator_id == 1 || true) && visibility == "PUBLIC"`, sql: "*", args: []any{"PUBLIC"}},
		{filter: `visibility == "PUBLIC" && (creator_id == 1 || true)`, sql: "*", args: []any{"PUBLIC"}},
		{filter: `(creator_id == 1 && false) || visibility == "PUBLIC"`, sql: "*", args: []any{"PUBLIC"}},
		{filter: `visibility == "PUBLIC" || (creator_id == 1 && false)`, sql: "*", args: []any{"PUBLIC"}},
		{filter: `!(creator_id == 1 || true) || visibility == "PUBLIC"`, sql: "*", args: []any{"PUBLIC"}},
		{filter: `(creator_id == 1 && true) || (tags.exists(t, t == "a") && false)`, sql: "*", args: []any{int64(1)}},
		{
			filter: `(visibility == "PRIVATE" || true) && (creator_id == 1 || true) && visibility == "PUBLIC" && creator_id == 2`,
			sql:    "*",
			args:   []any{"PUBLIC", int64(2)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.filter, func(t *testing.T) {
			stmt, err := engine.CompileToStatement(t.Context(), tc.filter)
			require.NoError(t, err)
			require.Equal(t, tc.args, stmt.Args)
			if tc.sql == "*" {
				require.NotContains(t, []string{"", "1 = 0"}, stmt.SQL)
			} else {
				require.Equal(t, tc.sql, stmt.SQL)
			}
			require.Equal(t, len(tc.args), strings.Count(stmt.SQL, "?"))
		})
	}
}

func TestCheckPlaceholderParityRejectsMismatch(t *testing.T) {
	t.Parallel()

	require.NoError(t, checkPlaceholderParity("a = ? AND b = ?", []any{1, 2}))
	require.Error(t, checkPlaceholderParity("a = ?", []any{1, 2}))
	require.Error(t, checkPlaceholderParity("a = ? AND b = ?", []any{1}))
}

// Store callers can append conditions after their own arguments.
func TestAppendConditionsPreservesArgumentOrder(t *testing.T) {
	t.Parallel()

	engine, err := DefaultEngine()
	require.NoError(t, err)
	where := []string{"memo.id = ?"}
	args := []any{int64(4)}
	filters := []string{`creator_id == 1 || true`, `pinned == true`, `creation_channel == "web"`}
	require.NoError(t, AppendConditions(t.Context(), engine, filters, &where, &args))
	require.Len(t, where, 3)
	require.Equal(t, []any{int64(4), 1, "web"}, args)
}
