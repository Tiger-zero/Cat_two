package filter

import (
	"context"
	"fmt"
	"strconv"
)

// TagBranchExpression matches a tag or any of its slash-delimited descendants.
// Some older memo payloads only store the literal child path.
func TagBranchExpression(path string) string {
	return "(" + strconv.Quote(path) + " in tags || tags.exists(t, t.startsWith(" + strconv.Quote(path+"/") + ")))"
}

// AppendConditions compiles the provided filters and appends the resulting SQL fragments and args.
func AppendConditions(ctx context.Context, engine *Engine, filters []string, dialect DialectName, where *[]string, args *[]any) error {
	for _, filterStr := range filters {
		stmt, err := engine.CompileToStatement(ctx, filterStr, RenderOptions{
			Dialect:           dialect,
			PlaceholderOffset: len(*args),
		})
		if err != nil {
			return err
		}
		if stmt.SQL == "" {
			continue
		}
		*where = append(*where, fmt.Sprintf("(%s)", stmt.SQL))
		*args = append(*args, stmt.Args...)
	}
	return nil
}
