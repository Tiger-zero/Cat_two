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
func AppendConditions(ctx context.Context, engine *Engine, filters []string, where *[]string, args *[]any) error {
	for _, filterStr := range filters {
		stmt, err := engine.CompileToStatement(ctx, filterStr)
		if err != nil {
			return err
		}
		appendStatement(stmt, where, args)
	}
	return nil
}

// AppendProgramCondition renders a previously compiled filter using the values
// fixed at compilation time. A nil program contributes no condition.
func AppendProgramCondition(program *Program, where *[]string, args *[]any) error {
	if program == nil {
		return nil
	}
	stmt, err := program.Render()
	if err != nil {
		return err
	}
	appendStatement(stmt, where, args)
	return nil
}

func appendStatement(stmt Statement, where *[]string, args *[]any) {
	if stmt.SQL == "" {
		return
	}
	*where = append(*where, fmt.Sprintf("(%s)", stmt.SQL))
	*args = append(*args, stmt.Args...)
}
