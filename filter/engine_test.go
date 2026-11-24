package filter

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestCompileAcceptsStandardTagEqualityPredicate(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	_, err = engine.Compile(context.Background(), `tags.exists(t, t == "1231")`)
	require.NoError(t, err)
}

func TestCompileRejectsLegacyNumericLogicalOperand(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	_, err = engine.Compile(context.Background(), `pinned && 1`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to compile filter")
}

func TestCompileRejectsNonBooleanTopLevelConstant(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	_, err = engine.Compile(context.Background(), `1`)
	require.EqualError(t, err, "filter must evaluate to a boolean value")
}

func TestCompileRejectsMalformedRegex(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	_, err = engine.Compile(context.Background(), `content.matches("(")`)
	require.Error(t, err)
}

func TestCompileRejectsStartsWithOnUnsupportedField(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	_, err = engine.Compile(context.Background(), `visibility.startsWith("P")`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support text matching")
}

func TestRetiredSpaceFilters(t *testing.T) {
	for _, engine := range []func() (*Engine, error){DefaultEngine, DefaultAttachmentEngine} {
		engine, err := engine()
		require.NoError(t, err)
		_, err = engine.Compile(context.Background(), `space == "spaces/team"`)
		require.ErrorContains(t, err, "undeclared reference to 'space'")
	}
}

func TestRenderAttachmentCreatorFilter(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewAttachmentSchema())
	require.NoError(t, err)
	statement, err := engine.CompileToStatement(context.Background(), `creator == "users/alice"`)
	require.NoError(t, err)
	require.Contains(t, statement.SQL, "CASE WHEN")
	require.Equal(t, []any{"users/alice"}, statement.Args)
}

func TestCompileContainsEscapesLikeWildcards(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	stmt, err := engine.CompileToStatement(context.Background(), `content.contains("50%_off")`)
	require.NoError(t, err)
	// The % and _ in the value must be escaped so they are matched literally,
	// and SQLite needs an explicit ESCAPE clause.
	require.Contains(t, stmt.SQL, `ESCAPE '\'`)
	require.Equal(t, []any{`%50\%\_off%`}, stmt.Args)
}

func TestRenderTagMembershipIsExact(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	const tag = `work_%"quoted"`
	fragments := []string{"json_each(", "COLLATE BINARY"}
	stmt, err := engine.CompileToStatement(context.Background(), `tag in ["work_%\"quoted\""]`)
	require.NoError(t, err)
	for _, fragment := range fragments {
		require.Contains(t, stmt.SQL, fragment)
	}
	require.NotContains(t, stmt.SQL, " LIKE ")
	require.Equal(t, []any{tag}, stmt.Args)
}

func TestRenderTagExistsEqualityIsExact(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	const tag = `work_%"quoted"`
	fragments := []string{"json_each(", "COLLATE BINARY"}
	stmt, err := engine.CompileToStatement(context.Background(), `tags.exists(t, t == "work_%\"quoted\"")`)
	require.NoError(t, err)
	for _, fragment := range fragments {
		require.Contains(t, stmt.SQL, fragment)
	}
	require.NotContains(t, stmt.SQL, " LIKE ")
	require.Equal(t, []any{tag}, stmt.Args)
}

func TestRenderTagStringPredicatesAreExact(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	predicates := []struct {
		name       string
		expression string
		sql        string
		args       []any
	}{
		{name: "equals", expression: `t == "Work_%!"`, sql: "COLLATE BINARY", args: []any{"Work_%!"}},
		{name: "startsWith", expression: `t.startsWith("Work_%!")`, sql: "instr(tag_item.value, ?) = 1", args: []any{"Work_%!"}},
		{name: "endsWith", expression: `t.endsWith("Work_%!")`, sql: "substr(tag_item.value, -length(?))", args: []any{"Work_%!", "Work_%!"}},
		{name: "contains", expression: `t.contains("Work_%!")`, sql: "instr(tag_item.value, ?) > 0", args: []any{"Work_%!"}},
	}

	for _, comprehension := range []struct {
		kind        string
		sqlFragment string
	}{
		{kind: "exists", sqlFragment: "EXISTS (SELECT 1"},
		{kind: "all", sqlFragment: "NOT EXISTS (SELECT 1"},
		{kind: "exists_one", sqlFragment: "SELECT COUNT(*)"},
	} {
		for _, predicate := range predicates {
			expression := fmt.Sprintf("tags.%s(t, %s)", comprehension.kind, predicate.expression)
			t.Run(comprehension.kind+"/"+predicate.name, func(t *testing.T) {
				t.Parallel()

				stmt, err := engine.CompileToStatement(context.Background(), expression)
				require.NoError(t, err)
				require.Contains(t, stmt.SQL, "json_each(")
				require.Contains(t, stmt.SQL, comprehension.sqlFragment)
				require.Contains(t, stmt.SQL, predicate.sql)
				require.NotContains(t, stmt.SQL, "memos_unicode_lower")
				require.Equal(t, predicate.args, stmt.Args)
			})
		}
	}
}

func TestRenderEmptyTagListIsFalse(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)
	stmt, err := engine.CompileToStatement(context.Background(), `tag in []`)
	require.NoError(t, err)
	require.Equal(t, "1 = 0", stmt.SQL)
	require.Empty(t, stmt.Args)
}

func TestRenderTagStringPredicatesSQLiteBehavior(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	_, err = db.Exec(`CREATE TABLE memo (id INTEGER PRIMARY KEY, payload TEXT)`)
	require.NoError(t, err)
	for _, fixture := range []struct {
		id      int
		payload string
	}{
		{1, `{"tags":["Work_%done"]}`},
		{2, `{"tags":["work_%done"]}`},
		{3, `{"tags":["WorkX%done"]}`},
		{4, `{"tags":["Work_XXdone"]}`},
		{5, `{"tags":["Work_%done","other"]}`},
		{6, `{"tags":["Work_%done","Work_%done"]}`},
		{7, `{"tags":["preWork_%done"]}`},
		{8, `{"tags":["Work_%doneSuffix"]}`},
	} {
		_, err = db.Exec(`INSERT INTO memo (id, payload) VALUES (?, ?)`, fixture.id, fixture.payload)
		require.NoError(t, err)
	}

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	tests := []struct {
		name      string
		predicate string
		exists    []int
		all       []int
		existsOne []int
	}{
		{
			name:      "equality",
			predicate: `t == "Work_%done"`,
			exists:    []int{1, 5, 6},
			all:       []int{1, 6},
			existsOne: []int{1, 5},
		},
		{
			name:      "startsWith",
			predicate: `t.startsWith("Work_%done")`,
			exists:    []int{1, 5, 6, 8},
			all:       []int{1, 6, 8},
			existsOne: []int{1, 5, 8},
		},
		{
			name:      "endsWith",
			predicate: `t.endsWith("Work_%done")`,
			exists:    []int{1, 5, 6, 7},
			all:       []int{1, 6, 7},
			existsOne: []int{1, 5, 7},
		},
		{
			name:      "contains",
			predicate: `t.contains("Work_%done")`,
			exists:    []int{1, 5, 6, 7, 8},
			all:       []int{1, 6, 7, 8},
			existsOne: []int{1, 5, 7, 8},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, comprehension := range []struct {
				name string
				want []int
			}{
				{name: "exists", want: tc.exists},
				{name: "all", want: tc.all},
				{name: "exists_one", want: tc.existsOne},
			} {
				t.Run(comprehension.name, func(t *testing.T) {
					stmt, err := engine.CompileToStatement(
						context.Background(),
						fmt.Sprintf("tags.%s(t, %s)", comprehension.name, tc.predicate),
					)
					require.NoError(t, err)
					require.Equal(t, comprehension.want, selectMemoIDs(t, db, stmt))
				})
			}
		})
	}
}

func selectMemoIDs(t *testing.T, db *sql.DB, stmt Statement) []int {
	t.Helper()

	rows, err := db.Query(`SELECT id FROM memo WHERE `+stmt.SQL+` ORDER BY id`, stmt.Args...)
	require.NoError(t, err)
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

func TestRenderStartsWith(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	fragments := []string{"memos_unicode_lower(", "`memo`.`content`", `ESCAPE '\'`}
	stmt, err := engine.CompileToStatement(context.Background(), `content.startsWith("TODO")`)
	require.NoError(t, err)
	for _, frag := range fragments {
		require.Contains(t, stmt.SQL, frag)
	}
	require.Equal(t, []any{"TODO%"}, stmt.Args)
}

func TestRenderEndsWith(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	stmt, err := engine.CompileToStatement(context.Background(), `content.endsWith(".md")`)
	require.NoError(t, err)
	require.Equal(t, []any{"%.md"}, stmt.Args)
}

func TestRenderMatches(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	fragment := "`memo`.`content` REGEXP ?"
	stmt, err := engine.CompileToStatement(context.Background(), `content.matches("v[0-9]+")`)
	require.NoError(t, err)
	require.Contains(t, stmt.SQL, fragment)
	require.Equal(t, []any{"v[0-9]+"}, stmt.Args)
}

func TestRenderTagsAll(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	fragments := []string{"NOT EXISTS", "json_each(", "json_array_length(", "instr(tag_item.value, ?) = 1"}
	args := []any{"work/"}
	stmt, err := engine.CompileToStatement(context.Background(), `tags.all(t, t.startsWith("work/"))`)
	require.NoError(t, err)
	for _, frag := range fragments {
		require.Contains(t, stmt.SQL, frag)
	}
	require.Equal(t, args, stmt.Args)
}

func TestRenderTextMatchEscaping(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	// Both % and _ in the value must be escaped so they match literally.
	stmt, err := engine.CompileToStatement(context.Background(), `content.contains("a%b_c")`)
	require.NoError(t, err)
	require.Equal(t, []any{`%a\%b\_c%`}, stmt.Args)
}

func TestRenderAllRejectsUnsupportedPredicate(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	// size() is not a valid per-element predicate inside all().
	_, err = engine.CompileToStatement(context.Background(), `tags.all(t, size(t) > 2)`)
	require.Error(t, err)
}

func TestRenderCreationChannel(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	sql := "COALESCE(NULLIF(json_extract(`memo`.`payload`, '$.creationChannel'), ''), 'unknown') = ?"
	stmt, err := engine.CompileToStatement(context.Background(), `creation_channel == "web"`)
	require.NoError(t, err)
	require.Equal(t, sql, stmt.SQL)
	require.Equal(t, []any{"web"}, stmt.Args)
}

func TestRenderHasLocation(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	sql := "JSON_EXTRACT(`memo`.`payload`, '$.location') IS NOT NULL"
	stmt, err := engine.CompileToStatement(context.Background(), `has_location`)
	require.NoError(t, err)
	require.Equal(t, sql, stmt.SQL)
	require.Empty(t, stmt.Args)
}

func TestRenderHasLocationNegationAndComparisons(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	const exists = "JSON_EXTRACT(`memo`.`payload`, '$.location') IS NOT NULL"
	cases := []struct {
		expr string
		sql  string
	}{
		{`has_location`, exists},
		{`!has_location`, "NOT (" + exists + ")"},
		{`has_location == true`, exists},
		{`has_location == false`, "NOT (" + exists + ")"},
		{`has_location != true`, "NOT (" + exists + ")"},
		{`has_location != false`, exists},
	}
	for _, tc := range cases {
		stmt, err := engine.CompileToStatement(context.Background(), tc.expr)
		require.NoError(t, err, tc.expr)
		require.Equal(t, tc.sql, stmt.SQL, tc.expr)
		require.Empty(t, stmt.Args, tc.expr)
	}
}

func TestCompileRejectsOrderingOnHasLocation(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	// Only ==/!= are meaningful for a presence flag.
	_, err = engine.Compile(context.Background(), `has_location < true`)
	require.Error(t, err)
}

// TestHasLocationSQLiteBehavior pins the presence semantics against a real
// database: a missing key, an explicit JSON null, and a NULL payload all count
// as absent, while any location object — even an empty one — counts as present.
func TestHasLocationSQLiteBehavior(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	_, err = db.Exec(`CREATE TABLE memo (id INTEGER PRIMARY KEY, payload TEXT)`)
	require.NoError(t, err)
	for _, fixture := range []struct {
		id      int
		payload any
	}{
		{1, `{}`},
		{2, `{"location":{"placeholder":"Tokyo","latitude":35.6,"longitude":139.7}}`},
		{3, `{"location":{}}`},
		{4, `{"location":null}`},
		{5, nil},
	} {
		_, err = db.Exec(`INSERT INTO memo (id, payload) VALUES (?, ?)`, fixture.id, fixture.payload)
		require.NoError(t, err)
	}

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	cases := []struct {
		expr string
		want []int
	}{
		{`has_location`, []int{2, 3}},
		{`!has_location`, []int{1, 4, 5}},
		{`has_location == false`, []int{1, 4, 5}},
		{`has_location != false`, []int{2, 3}},
	}
	for _, tc := range cases {
		stmt, err := engine.CompileToStatement(context.Background(), tc.expr)
		require.NoError(t, err, tc.expr)
		require.Equal(t, tc.want, selectMemoIDs(t, db, stmt), tc.expr)
	}
}

func TestRenderJSONBoolComparisons(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	const predicate = "JSON_EXTRACT(`memo`.`payload`, '$.property.hasLink') IS TRUE"
	cases := []struct {
		expr string
		sql  string
	}{
		{`has_link`, predicate},
		{`!has_link`, "NOT (" + predicate + ")"},
		{`has_link == true`, predicate},
		{`has_link == false`, "NOT (" + predicate + ")"},
		{`has_link != true`, "NOT (" + predicate + ")"},
		{`has_link != false`, predicate},
	}
	for _, tc := range cases {
		stmt, err := engine.CompileToStatement(context.Background(), tc.expr)
		require.NoError(t, err, tc.expr)
		require.Equal(t, tc.sql, stmt.SQL, tc.expr)
		require.Empty(t, stmt.Args, tc.expr)
	}
}

// TestJSONBoolComparisonSQLiteBehavior pins the comparison semantics of the
// JSON boolean flags against a real database. Memo payloads omit false flags,
// so a missing key, a missing property object and a NULL payload all count as
// false, and `flag == false` must match the same rows as `!flag`.
func TestJSONBoolComparisonSQLiteBehavior(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	_, err = db.Exec(`CREATE TABLE memo (id INTEGER PRIMARY KEY, payload TEXT)`)
	require.NoError(t, err)

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	flags := map[string]string{
		"has_task_list":        "hasTaskList",
		"has_link":             "hasLink",
		"has_code":             "hasCode",
		"has_incomplete_tasks": "hasIncompleteTasks",
	}
	for field, key := range flags {
		_, err = db.Exec(`DELETE FROM memo`)
		require.NoError(t, err)
		for _, fixture := range []struct {
			id      int
			payload any
		}{
			{1, `{}`},
			{2, fmt.Sprintf(`{"property":{"%s":true}}`, key)},
			{3, fmt.Sprintf(`{"property":{"%s":false}}`, key)},
			{4, `{"property":{}}`},
			{5, nil},
		} {
			_, err = db.Exec(`INSERT INTO memo (id, payload) VALUES (?, ?)`, fixture.id, fixture.payload)
			require.NoError(t, err)
		}

		cases := []struct {
			expr string
			want []int
		}{
			{field, []int{2}},
			{"!" + field, []int{1, 3, 4, 5}},
			{field + " == true", []int{2}},
			{field + " == false", []int{1, 3, 4, 5}},
			{field + " != true", []int{1, 3, 4, 5}},
			{field + " != false", []int{2}},
		}
		for _, tc := range cases {
			stmt, err := engine.CompileToStatement(context.Background(), tc.expr)
			require.NoError(t, err, tc.expr)
			require.Equal(t, tc.want, selectMemoIDs(t, db, stmt), tc.expr)
		}
	}
}

func TestRenderExistsComparisons(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	predicates := map[string]string{
		"has_attachments": "EXISTS (SELECT 1 FROM `attachment` WHERE `attachment`.`memo_id` = `memo`.`id`)",
		"has_image": "(JSON_EXTRACT(`memo`.`payload`, '$.property.hasInlineImage') IS TRUE OR " +
			"EXISTS (SELECT 1 FROM `attachment` WHERE `attachment`.`memo_id` = `memo`.`id` AND LOWER(`attachment`.`type`) LIKE 'image/%'))",
	}
	for field, predicate := range predicates {
		cases := []struct {
			expr string
			sql  string
		}{
			{field, predicate},
			{"!" + field, "NOT (" + predicate + ")"},
			{field + " == true", predicate},
			{field + " == false", "NOT (" + predicate + ")"},
			{field + " != true", "NOT (" + predicate + ")"},
			{field + " != false", predicate},
		}
		for _, tc := range cases {
			stmt, err := engine.CompileToStatement(context.Background(), tc.expr)
			require.NoError(t, err, tc.expr)
			require.Equal(t, tc.sql, stmt.SQL, tc.expr)
			require.Empty(t, stmt.Args, tc.expr)
		}
	}
}

func TestRenderExistsRejectsUnsupportedOperators(t *testing.T) {
	t.Parallel()

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)
	for _, expr := range []string{`has_image > false`, `has_attachments == "yes"`, `"image" in has_image`} {
		_, err := engine.CompileToStatement(context.Background(), expr)
		require.Error(t, err, expr)
	}
}

// TestExistsSQLiteBehavior pins has_attachments and has_image against real
// tables: has_attachments follows attachment.memo_id alone, and has_image is
// true for Markdown image syntax or an attached image/* file, whatever the
// MIME type's case, and false for other attachments and unbound uploads.
func TestExistsSQLiteBehavior(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	_, err = db.Exec(`CREATE TABLE memo (id INTEGER PRIMARY KEY, payload TEXT)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE attachment (id INTEGER PRIMARY KEY, memo_id INTEGER, type TEXT NOT NULL DEFAULT '')`)
	require.NoError(t, err)

	for _, fixture := range []struct {
		id      int
		payload any
	}{
		{1, `{}`}, // nothing
		{2, `{"property":{"hasInlineImage":true}}`}, // external image in content, no attachment
		{3, `{}`}, // uploaded photo, not referenced in content
		{4, `{}`}, // PDF attachment only
		{5, `{"property":{"hasInlineImage":true}}`}, // managed inline image, also bound
		{6, nil}, // NULL payload, upper-case MIME image
		{7, `{"property":{"hasInlineImage":false}}`}, // explicit false flag, nothing attached
	} {
		_, err = db.Exec(`INSERT INTO memo (id, payload) VALUES (?, ?)`, fixture.id, fixture.payload)
		require.NoError(t, err)
	}
	for _, attachment := range []struct {
		memoID   any
		mimeType string
	}{
		{3, "image/jpeg"},
		{4, "application/pdf"},
		{5, "image/png"},
		{6, "IMAGE/HEIC"},
		{nil, "image/png"}, // uploaded but not bound to any memo
	} {
		_, err = db.Exec(`INSERT INTO attachment (memo_id, type) VALUES (?, ?)`, attachment.memoID, attachment.mimeType)
		require.NoError(t, err)
	}

	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	cases := []struct {
		expr string
		want []int
	}{
		{`has_attachments`, []int{3, 4, 5, 6}},
		{`!has_attachments`, []int{1, 2, 7}},
		{`has_attachments == false`, []int{1, 2, 7}},
		{`has_image`, []int{2, 3, 5, 6}},
		{`!has_image`, []int{1, 4, 7}},
		{`has_image != true`, []int{1, 4, 7}},
		{`has_attachments && !has_image`, []int{4}},
		{`has_image && !has_attachments`, []int{2}},
	}
	for _, tc := range cases {
		stmt, err := engine.CompileToStatement(context.Background(), tc.expr)
		require.NoError(t, err, tc.expr)
		require.Equal(t, tc.want, selectMemoIDs(t, db, stmt), tc.expr)
	}
}

func TestCompileRejectsOversizedExpression(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine(NewSchema())
	require.NoError(t, err)

	var builder strings.Builder
	builder.WriteString(`content.contains("a")`)
	for builder.Len() < maxFilterExpressionSize+64 {
		builder.WriteString(` || content.contains("a")`)
	}
	_, err = engine.Compile(context.Background(), builder.String())
	require.Error(t, err)

	_, err = engine.Compile(context.Background(), `content.contains("a") || content.contains("b")`)
	require.NoError(t, err)
}
