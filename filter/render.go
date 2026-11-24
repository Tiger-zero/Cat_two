package filter

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type renderer struct {
	schema Schema
	args   []any
}

type renderResult struct {
	sql string
}

func (r *renderer) Render(cond Condition) (Statement, error) {
	// Fold boolean constants first: a subtree that folds away must never reach
	// the renderer, or its arguments would outlive its discarded SQL and shift
	// every later positional bind.
	cond = foldConstants(cond)
	if c, ok := cond.(*ConstantCondition); ok {
		if c.Value {
			return Statement{SQL: "", Args: []any{}}, nil
		}
		return Statement{SQL: "1 = 0", Args: []any{}}, nil
	}

	result, err := r.renderCondition(cond)
	if err != nil {
		return Statement{}, err
	}
	args := r.args
	if args == nil {
		args = []any{}
	}
	if err := checkPlaceholderParity(result.sql, args); err != nil {
		return Statement{}, err
	}
	return Statement{SQL: result.sql, Args: args}, nil
}

// foldConstants removes boolean constants from the condition tree, so every
// subtree the renderer visits contributes its SQL and its arguments together.
// The result is either a single ConstantCondition or a tree with no constants.
func foldConstants(cond Condition) Condition {
	switch c := cond.(type) {
	case *LogicalCondition:
		left := foldConstants(c.Left)
		right := foldConstants(c.Right)
		leftConst, leftOK := left.(*ConstantCondition)
		rightConst, rightOK := right.(*ConstantCondition)
		// AND is decided by a false operand, OR by a true one; the other
		// constant is the operator's identity and drops out.
		decisive := c.Operator == LogicalOr
		if (leftOK && leftConst.Value == decisive) || (rightOK && rightConst.Value == decisive) {
			return &ConstantCondition{Value: decisive}
		}
		if leftOK {
			return right
		}
		if rightOK {
			return left
		}
		return &LogicalCondition{Operator: c.Operator, Left: left, Right: right}
	case *NotCondition:
		child := foldConstants(c.Expr)
		if childConst, ok := child.(*ConstantCondition); ok {
			return &ConstantCondition{Value: !childConst.Value}
		}
		return &NotCondition{Expr: child}
	default:
		return cond
	}
}

// checkPlaceholderParity rejects a fragment whose placeholders and arguments
// disagree. Callers bind positionally after appending their own predicates, so
// a surplus argument would silently bind to a later placeholder.
func checkPlaceholderParity(sql string, args []any) error {
	if count := strings.Count(sql, "?"); count != len(args) {
		return errors.Errorf("filter rendered %d placeholders for %d arguments", count, len(args))
	}
	return nil
}

func (r *renderer) renderCondition(cond Condition) (renderResult, error) {
	switch c := cond.(type) {
	case *LogicalCondition:
		return r.renderLogicalCondition(c)
	case *NotCondition:
		return r.renderNotCondition(c)
	case *FieldPredicateCondition:
		return r.renderFieldPredicate(c)
	case *ComparisonCondition:
		return r.renderComparison(c)
	case *InCondition:
		return r.renderInCondition(c)
	case *ElementInCondition:
		return r.renderElementInCondition(c)
	case *TextMatchCondition:
		return r.renderTextMatch(c)
	case *RegexCondition:
		return r.renderRegex(c)
	case *ListComprehensionCondition:
		return r.renderListComprehension(c)
	case *ConstantCondition:
		return renderResult{}, errors.New("constant condition must be folded before rendering")
	default:
		return renderResult{}, errors.Errorf("unsupported condition type %T", c)
	}
}

func (r *renderer) renderLogicalCondition(cond *LogicalCondition) (renderResult, error) {
	left, err := r.renderCondition(cond.Left)
	if err != nil {
		return renderResult{}, err
	}
	right, err := r.renderCondition(cond.Right)
	if err != nil {
		return renderResult{}, err
	}

	switch cond.Operator {
	case LogicalAnd, LogicalOr:
		return renderResult{sql: fmt.Sprintf("(%s %s %s)", left.sql, cond.Operator, right.sql)}, nil
	default:
		return renderResult{}, errors.Errorf("unsupported logical operator %s", cond.Operator)
	}
}

func (r *renderer) renderNotCondition(cond *NotCondition) (renderResult, error) {
	child, err := r.renderCondition(cond.Expr)
	if err != nil {
		return renderResult{}, err
	}

	return renderResult{
		sql: fmt.Sprintf("NOT (%s)", child.sql),
	}, nil
}

func (r *renderer) renderFieldPredicate(cond *FieldPredicateCondition) (renderResult, error) {
	field, ok := r.schema.Field(cond.Field)
	if !ok {
		return renderResult{}, errors.Errorf("unknown field %q", cond.Field)
	}

	switch field.Kind {
	case FieldKindBoolColumn:
		column := qualifyColumn(field.Column)
		return renderResult{
			sql: fmt.Sprintf("%s IS TRUE", column),
		}, nil
	case FieldKindJSONBool:
		return renderResult{sql: jsonBoolPredicate(field)}, nil
	case FieldKindJSONExists:
		return renderResult{sql: jsonExistsSQL(field)}, nil
	case FieldKindExists:
		sql, err := existsPredicate(field)
		if err != nil {
			return renderResult{}, errors.Wrap(err, "failed to render existence predicate")
		}
		return renderResult{sql: sql}, nil
	default:
		return renderResult{}, errors.Errorf("field %q cannot be used as a predicate", cond.Field)
	}
}

func (r *renderer) renderComparison(cond *ComparisonCondition) (renderResult, error) {
	switch left := cond.Left.(type) {
	case *FieldRef:
		field, ok := r.schema.Field(left.Name)
		if !ok {
			return renderResult{}, errors.Errorf("unknown field %q", left.Name)
		}
		switch field.Kind {
		case FieldKindBoolColumn:
			return r.renderBoolColumnComparison(field, cond.Operator, cond.Right)
		case FieldKindJSONBool:
			return renderJSONBoolComparison(field, cond.Operator, cond.Right)
		case FieldKindJSONExists:
			return renderJSONExistsComparison(field, cond.Operator, cond.Right)
		case FieldKindExists:
			return renderExistsComparison(field, cond.Operator, cond.Right)
		case FieldKindScalar:
			return r.renderScalarComparison(field, cond.Operator, cond.Right)
		default:
			return renderResult{}, errors.Errorf("field %q does not support comparison", field.Name)
		}
	case *FunctionValue:
		return r.renderFunctionComparison(left, cond.Operator, cond.Right)
	case *FieldAccessorValue:
		return r.renderAccessorComparison(left, cond.Operator, cond.Right)
	default:
		return renderResult{}, errors.New("comparison must start with a field reference or supported function")
	}
}

// accessorSpec maps a CEL accessor to its strftime format and the offset
// needed for its zero-based fields, such as the month.
type accessorSpec struct {
	format string
	offset int
}

var accessorSpecs = map[string]accessorSpec{
	"getFullYear":   {"%Y", 0},
	"getMonth":      {"%m", 1},
	"getDate":       {"%d", 0},
	"getDayOfMonth": {"%d", 1},
	"getDayOfWeek":  {"%w", 0},
	"getDayOfYear":  {"%j", 1},
	"getHours":      {"%H", 0},
	"getMinutes":    {"%M", 0},
	"getSeconds":    {"%S", 0},
}

func (r *renderer) renderAccessorComparison(acc *FieldAccessorValue, op ComparisonOperator, right ValueExpr) (renderResult, error) {
	field, ok := r.schema.Field(acc.Field)
	if !ok {
		return renderResult{}, errors.Errorf("unknown field %q", acc.Field)
	}
	value, err := expectNumericLiteral(right)
	if err != nil {
		return renderResult{}, err
	}
	expr, err := timestampAccessorExpr(field, acc.Accessor)
	if err != nil {
		return renderResult{}, err
	}
	placeholder := r.addArg(value)
	return renderResult{
		sql: fmt.Sprintf("%s %s %s", expr, sqlOperator(op), placeholder),
	}, nil
}

// timestampAccessorExpr extracts a UTC date part from an epoch column.
func timestampAccessorExpr(field Field, accessor string) (string, error) {
	spec, ok := accessorSpecs[accessor]
	if !ok {
		return "", errors.Errorf("unsupported timestamp accessor %q", accessor)
	}
	base := fmt.Sprintf("CAST(strftime('%s', %s, 'unixepoch') AS INTEGER)", spec.format, qualifyColumn(field.Column))
	if spec.offset != 0 {
		base = fmt.Sprintf("(%s - %d)", base, spec.offset)
	}
	return base, nil
}

func (r *renderer) renderFunctionComparison(fn *FunctionValue, op ComparisonOperator, right ValueExpr) (renderResult, error) {
	if fn.Name != "size" {
		return renderResult{}, errors.Errorf("unsupported function %s in comparison", fn.Name)
	}
	if len(fn.Args) != 1 {
		return renderResult{}, errors.New("size() expects one argument")
	}
	fieldArg, ok := fn.Args[0].(*FieldRef)
	if !ok {
		return renderResult{}, errors.New("size() argument must be a field")
	}

	field, ok := r.schema.Field(fieldArg.Name)
	if !ok {
		return renderResult{}, errors.Errorf("unknown field %q", fieldArg.Name)
	}
	if field.Kind == FieldKindVirtualAlias {
		field, ok = r.schema.ResolveAlias(fieldArg.Name)
		if !ok {
			return renderResult{}, errors.Errorf("invalid alias %q", fieldArg.Name)
		}
	}

	value, err := expectNumericLiteral(right)
	if err != nil {
		return renderResult{}, err
	}

	var expr string
	switch {
	case field.Kind == FieldKindJSONList:
		expr = jsonArrayLengthExpr(field)
	case field.Kind == FieldKindScalar && field.Type == FieldTypeString:
		expr = fmt.Sprintf("LENGTH(%s)", field.columnExpr())
	default:
		return renderResult{}, errors.Errorf("size() does not support field %q", field.Name)
	}

	placeholder := r.addArg(value)
	return renderResult{
		sql: fmt.Sprintf("%s %s %s", expr, sqlOperator(op), placeholder),
	}, nil
}

func (r *renderer) renderScalarComparison(field Field, op ComparisonOperator, right ValueExpr) (renderResult, error) {
	lit, err := expectLiteral(right)
	if err != nil {
		return renderResult{}, err
	}

	columnExpr := field.columnExpr()
	if lit == nil {
		switch op {
		case CompareEq:
			return renderResult{sql: fmt.Sprintf("%s IS NULL", columnExpr)}, nil
		case CompareNeq:
			return renderResult{sql: fmt.Sprintf("%s IS NOT NULL", columnExpr)}, nil
		default:
			return renderResult{}, errors.Errorf("operator %s not supported for null comparison", op)
		}
	}

	placeholder := ""
	switch field.Type {
	case FieldTypeString:
		value, ok := lit.(string)
		if !ok {
			return renderResult{}, errors.Errorf("field %q expects string value", field.Name)
		}
		placeholder = r.addArg(value)
	case FieldTypeInt, FieldTypeTimestamp:
		num, err := toInt64(lit)
		if err != nil {
			return renderResult{}, errors.Wrapf(err, "field %q expects integer value", field.Name)
		}
		placeholder = r.addArg(num)
	default:
		return renderResult{}, errors.Errorf("unsupported data type %q for field %s", field.Type, field.Name)
	}

	return renderResult{
		sql: fmt.Sprintf("%s %s %s", columnExpr, sqlOperator(op), placeholder),
	}, nil
}

func (r *renderer) renderBoolColumnComparison(field Field, op ComparisonOperator, right ValueExpr) (renderResult, error) {
	value, err := expectBool(right)
	if err != nil {
		return renderResult{}, err
	}
	placeholder := r.addBoolArg(value)
	column := qualifyColumn(field.Column)
	return renderResult{
		sql: fmt.Sprintf("%s %s %s", column, sqlOperator(op), placeholder),
	}, nil
}

// renderJSONBoolComparison compares a JSON boolean flag with a literal. Memo
// payloads omit false flags, so a missing key means false: comparisons reuse
// jsonBoolPredicate, which already treats a missing key or a NULL payload as
// false.
func renderJSONBoolComparison(field Field, op ComparisonOperator, right ValueExpr) (renderResult, error) {
	value, err := expectBool(right)
	if err != nil {
		return renderResult{}, errors.Wrap(err, "json boolean comparison requires a boolean value")
	}
	return renderPredicateComparison(field, op, value, jsonBoolPredicate(field))
}

func (r *renderer) renderInCondition(cond *InCondition) (renderResult, error) {
	fieldRef, ok := cond.Left.(*FieldRef)
	if !ok {
		return renderResult{}, errors.New("IN operator requires a field on the left-hand side")
	}

	if fieldRef.Name == "tag" {
		return r.renderTagInList(cond.Values)
	}

	field, ok := r.schema.Field(fieldRef.Name)
	if !ok {
		return renderResult{}, errors.Errorf("unknown field %q", fieldRef.Name)
	}

	if field.Kind != FieldKindScalar {
		return renderResult{}, errors.Errorf("field %q does not support IN()", fieldRef.Name)
	}

	return r.renderScalarInCondition(field, cond.Values)
}

func (r *renderer) renderTagInList(values []ValueExpr) (renderResult, error) {
	field, ok := r.schema.ResolveAlias("tag")
	if !ok {
		return renderResult{}, errors.New("tag attribute is not configured")
	}

	conditions := make([]string, 0, len(values))
	for _, v := range values {
		lit, err := expectLiteral(v)
		if err != nil {
			return renderResult{}, err
		}
		str, ok := lit.(string)
		if !ok {
			return renderResult{}, errors.New("tags must be compared with string literals")
		}

		condition, err := r.renderJSONListContains(field, str)
		if err != nil {
			return renderResult{}, err
		}
		conditions = append(conditions, condition.sql)
	}

	if len(conditions) == 0 {
		return renderResult{sql: "1 = 0"}, nil
	}
	if len(conditions) == 1 {
		return renderResult{sql: conditions[0]}, nil
	}
	return renderResult{
		sql: fmt.Sprintf("(%s)", strings.Join(conditions, " OR ")),
	}, nil
}

func (r *renderer) renderElementInCondition(cond *ElementInCondition) (renderResult, error) {
	field, ok := r.schema.Field(cond.Field)
	if !ok {
		return renderResult{}, errors.Errorf("unknown field %q", cond.Field)
	}
	if field.Kind != FieldKindJSONList {
		return renderResult{}, errors.Errorf("field %q is not a tag list", cond.Field)
	}

	lit, err := expectLiteral(cond.Element)
	if err != nil {
		return renderResult{}, err
	}
	str, ok := lit.(string)
	if !ok {
		return renderResult{}, errors.New("tags membership requires string literal")
	}

	return r.renderJSONListContains(field, str)
}

func (r *renderer) renderJSONListContains(field Field, value string) (renderResult, error) {
	return r.renderTagComprehension(field, &EqualsPredicate{Value: value}, ComprehensionExists)
}

func (r *renderer) renderScalarInCondition(field Field, values []ValueExpr) (renderResult, error) {
	placeholders := make([]string, 0, len(values))

	for _, v := range values {
		lit, err := expectLiteral(v)
		if err != nil {
			return renderResult{}, err
		}
		switch field.Type {
		case FieldTypeString:
			str, ok := lit.(string)
			if !ok {
				return renderResult{}, errors.Errorf("field %q expects string values", field.Name)
			}
			placeholders = append(placeholders, r.addArg(str))
		case FieldTypeInt:
			num, err := toInt64(lit)
			if err != nil {
				return renderResult{}, err
			}
			placeholders = append(placeholders, r.addArg(num))
		default:
			return renderResult{}, errors.Errorf("field %q does not support IN() comparisons", field.Name)
		}
	}

	column := field.columnExpr()
	return renderResult{
		sql: fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ",")),
	}, nil
}

func (r *renderer) renderTextMatch(cond *TextMatchCondition) (renderResult, error) {
	field, ok := r.schema.Field(cond.Field)
	if !ok {
		return renderResult{}, errors.Errorf("unknown field %q", cond.Field)
	}
	column := field.columnExpr()
	pattern := likePattern(cond.Mode, cond.Value)
	return renderResult{sql: r.foldedLike(column, pattern)}, nil
}

func (r *renderer) renderRegex(cond *RegexCondition) (renderResult, error) {
	field, ok := r.schema.Field(cond.Field)
	if !ok {
		return renderResult{}, errors.Errorf("unknown field %q", cond.Field)
	}
	column := field.columnExpr()
	// SQLite uses the registered regexp() function.
	return renderResult{sql: fmt.Sprintf("%s REGEXP %s", column, r.addArg(cond.Pattern))}, nil
}

// foldedLike matches a literal pattern using Unicode-aware case folding.
func (r *renderer) foldedLike(colExpr, pattern string) string {
	return fmt.Sprintf(`memos_unicode_lower(%s) LIKE memos_unicode_lower(%s) ESCAPE '\'`, colExpr, r.addArg(pattern))
}

// likePattern escapes LIKE metacharacters in value and wraps it for the mode.
func likePattern(mode TextMatchMode, value string) string {
	escaped := escapeLikeLiteral(value)
	switch mode {
	case TextMatchPrefix:
		return escaped + "%"
	case TextMatchSuffix:
		return "%" + escaped
	default:
		return "%" + escaped + "%"
	}
}

// escapeLikeLiteral escapes the LIKE metacharacters \, %, and _ so user input
// is matched literally. The generated SQLite predicates declare backslash as their escape character.
func escapeLikeLiteral(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *renderer) renderListComprehension(cond *ListComprehensionCondition) (renderResult, error) {
	field, ok := r.schema.Field(cond.Field)
	if !ok {
		return renderResult{}, errors.Errorf("unknown field %q", cond.Field)
	}

	if field.Kind != FieldKindJSONList {
		return renderResult{}, errors.Errorf("field %q is not a JSON list", cond.Field)
	}

	return r.renderTagComprehension(field, cond.Predicate, cond.Kind)
}

// renderTagComprehension evaluates every tag predicate against individual JSON
// string elements. This avoids JSON text matching and gives all comprehension
// kinds the same exact, case-sensitive semantics.
func (r *renderer) renderTagComprehension(field Field, pred PredicateExpr, kind ComprehensionKind) (renderResult, error) {
	arrayExpr := jsonExtractExpr(field)
	elemCond, err := r.tagElementPredicateSQL("tag_item.value", pred)
	if err != nil {
		return renderResult{}, err
	}

	arrayExpr = fmt.Sprintf("COALESCE(%s, JSON_ARRAY())", arrayExpr)
	elements := fmt.Sprintf("json_each(%s) AS tag_item", arrayExpr)

	switch kind {
	case ComprehensionExists:
		return renderResult{sql: fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE %s)", elements, elemCond)}, nil
	case ComprehensionAll:
		return renderResult{
			sql: fmt.Sprintf("(json_array_length(%s) > 0 AND NOT EXISTS (SELECT 1 FROM %s WHERE NOT (%s)))", arrayExpr, elements, elemCond),
		}, nil
	case ComprehensionExistsOne:
		return renderResult{sql: fmt.Sprintf("(SELECT COUNT(*) FROM %s WHERE %s) = 1", elements, elemCond)}, nil
	default:
		return renderResult{}, errors.Errorf("unsupported comprehension kind %s", kind)
	}
}

// tagElementPredicateSQL matches one tag case-sensitively. SQLite's LIKE folds
// ASCII case, so prefix, suffix and substring checks use string functions.
func (r *renderer) tagElementPredicateSQL(element string, pred PredicateExpr) (string, error) {
	switch p := pred.(type) {
	case *EqualsPredicate:
		return fmt.Sprintf("(%s COLLATE BINARY) = (%s COLLATE BINARY)", element, r.addArg(p.Value)), nil
	case *StartsWithPredicate:
		return r.tagElementTextMatch(element, TextMatchPrefix, p.Prefix)
	case *EndsWithPredicate:
		return r.tagElementTextMatch(element, TextMatchSuffix, p.Suffix)
	case *ContainsPredicate:
		return r.tagElementTextMatch(element, TextMatchContains, p.Substring)
	default:
		return "", errors.Errorf("unsupported tag predicate %T", pred)
	}
}

func (r *renderer) tagElementTextMatch(element string, mode TextMatchMode, value string) (string, error) {
	if value == "" {
		return "1 = 1", nil
	}
	switch mode {
	case TextMatchPrefix:
		return fmt.Sprintf("instr(%s, %s) = 1", element, r.addArg(value)), nil
	case TextMatchSuffix:
		lengthValue := r.addArg(value)
		compareValue := r.addArg(value)
		return fmt.Sprintf("(substr(%s, -length(%s)) COLLATE BINARY) = (%s COLLATE BINARY)", element, lengthValue, compareValue), nil
	case TextMatchContains:
		return fmt.Sprintf("instr(%s, %s) > 0", element, r.addArg(value)), nil
	default:
		return "", errors.Errorf("unsupported tag text match mode %s", mode)
	}
}

// jsonExistsSQL treats a missing key and an explicit JSON null as absent.
func jsonExistsSQL(field Field) string {
	return fmt.Sprintf("%s IS NOT NULL", jsonExtractExpr(field))
}

func renderJSONExistsComparison(field Field, op ComparisonOperator, right ValueExpr) (renderResult, error) {
	value, err := expectBool(right)
	if err != nil {
		return renderResult{}, errors.Wrap(err, "json existence comparison requires a boolean value")
	}
	return renderPredicateComparison(field, op, value, jsonExistsSQL(field))
}

// existsPredicate renders a FieldKindExists field: an EXISTS subquery over the
// related rows, OR-ed with the field's JSON boolean flag when it has one. The
// flag goes through jsonBoolPredicate, so a missing key or NULL payload reads
// as false and the whole expression stays two-valued under NOT.
func existsPredicate(field Field) (string, error) {
	related := field.Related
	if related == nil {
		return "", errors.Errorf("field %q has no related rows", field.Name)
	}
	where := fmt.Sprintf("%s = %s",
		qualifyColumn(Column{Table: related.Table, Name: related.ForeignKey}),
		qualifyColumn(related.Owner))
	if related.PrefixColumn != "" {
		if related.Prefix == "" || strings.ContainsAny(related.Prefix, `'"%_\`) {
			return "", errors.Errorf("field %q has an unsafe prefix %q", field.Name, related.Prefix)
		}
		where += fmt.Sprintf(" AND LOWER(%s) LIKE '%s%%'",
			qualifyColumn(Column{Table: related.Table, Name: related.PrefixColumn}),
			strings.ToLower(related.Prefix))
	}
	exists := fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE %s)", quoteTable(related.Table), where)
	if len(field.JSONPath) == 0 {
		return exists, nil
	}
	return fmt.Sprintf("(%s OR %s)", jsonBoolPredicate(field), exists), nil
}

func renderExistsComparison(field Field, op ComparisonOperator, right ValueExpr) (renderResult, error) {
	value, err := expectBool(right)
	if err != nil {
		return renderResult{}, errors.Wrap(err, "existence comparison requires a boolean value")
	}
	predicate, err := existsPredicate(field)
	if err != nil {
		return renderResult{}, errors.Wrap(err, "failed to render existence comparison")
	}
	return renderPredicateComparison(field, op, value, predicate)
}

// renderPredicateComparison renders `field == value` or `field != value` for a
// field whose truth is fully described by predicate: the predicate itself when
// the comparison asks for true, its negation when it asks for false.
func renderPredicateComparison(field Field, op ComparisonOperator, value bool, predicate string) (renderResult, error) {
	want := value
	switch op {
	case CompareEq:
	case CompareNeq:
		want = !want
	default:
		return renderResult{}, errors.Errorf("operator %s not supported for field %q", op, field.Name)
	}
	if want {
		return renderResult{sql: predicate}, nil
	}
	return renderResult{sql: fmt.Sprintf("NOT (%s)", predicate)}, nil
}

func jsonBoolPredicate(field Field) string {
	return fmt.Sprintf("%s IS TRUE", jsonExtractExpr(field))
}

func (r *renderer) addArg(value any) string {
	r.args = append(r.args, value)
	return "?"
}

func (r *renderer) addBoolArg(value bool) string {
	if value {
		return r.addArg(1)
	}
	return r.addArg(0)
}

func expectLiteral(expr ValueExpr) (any, error) {
	lit, ok := expr.(*LiteralValue)
	if !ok {
		return nil, errors.New("expression must be a literal")
	}
	return lit.Value, nil
}

func expectBool(expr ValueExpr) (bool, error) {
	lit, err := expectLiteral(expr)
	if err != nil {
		return false, err
	}
	value, ok := lit.(bool)
	if !ok {
		return false, errors.New("boolean literal required")
	}
	return value, nil
}

func expectNumericLiteral(expr ValueExpr) (int64, error) {
	lit, err := expectLiteral(expr)
	if err != nil {
		return 0, err
	}
	return toInt64(lit)
}

func toInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint32:
		return int64(v), nil
	case uint64:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case float64:
		return int64(v), nil
	default:
		return 0, errors.Errorf("cannot convert %T to int64", value)
	}
}

func sqlOperator(op ComparisonOperator) string {
	return string(op)
}

func quoteTable(table string) string {
	return fmt.Sprintf("`%s`", table)
}

func qualifyColumn(col Column) string {
	return fmt.Sprintf("`%s`.`%s`", col.Table, col.Name)
}

func jsonExtractExpr(field Field) string {
	return fmt.Sprintf("JSON_EXTRACT(%s, '$.%s')", qualifyColumn(field.Column), strings.Join(field.JSONPath, "."))
}

func jsonArrayLengthExpr(field Field) string {
	return fmt.Sprintf("JSON_ARRAY_LENGTH(COALESCE(%s, JSON_ARRAY()))", jsonExtractExpr(field))
}
